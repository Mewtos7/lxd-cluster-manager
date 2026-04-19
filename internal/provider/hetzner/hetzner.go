// Package hetzner provides the Hetzner Cloud implementation of the
// HyperscalerProvider interface (ADR-005). Server provisioning is managed via
// the Pulumi Automation API (pulumi-hcloud provider) embedded in the manager
// binary. Read and delete operations use the Hetzner Cloud REST API directly
// (hcloud-go) because those operations do not require Pulumi's declarative
// model.
//
// The provider uses [pulumiruntime.Runtime] to drive Pulumi stacks in-process.
// Wire a runtime at construction time via [WithRuntime]. Without a runtime the
// ProvisionServer operation returns an error directing the operator to configure
// one.
package hetzner

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	hcloudpulumi "github.com/pulumi/pulumi-hcloud/sdk/go/hcloud"
	gopulumi "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	hcloud "github.com/hetznercloud/hcloud-go/v2/hcloud"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	pulumiruntime "github.com/Mewtos7/lx-container-weaver/internal/pulumi"
)

// Provider is the Hetzner Cloud implementation of provider.HyperscalerProvider.
type Provider struct {
	// apiToken is the Hetzner Cloud API token used by the Pulumi hcloud
	// provider for authentication and by the hcloud REST client.
	apiToken string

	// client is the Hetzner Cloud REST API client used for read and delete
	// operations that do not require Pulumi's declarative provisioning model.
	client *hcloud.Client

	// runtime drives Pulumi stack lifecycle for ProvisionServer operations.
	// Nil when the provider is used without a wired runtime.
	runtime *pulumiruntime.Runtime
}

// Option is a functional option for configuring a [Provider] at construction.
type Option func(*Provider)

// WithRuntime wires a [pulumiruntime.Runtime] into the Provider so that
// [Provider.ProvisionServer] can manage cloud resources in-process via the
// Pulumi Automation API (ADR-005).
//
// If this option is not provided, ProvisionServer returns an error directing
// the operator to configure a runtime.
func WithRuntime(rt *pulumiruntime.Runtime) Option {
	return func(p *Provider) { p.runtime = rt }
}

// New creates a new Hetzner Provider authenticated with the given API token.
// Use [WithRuntime] to attach a Pulumi Automation runtime for provisioning.
func New(apiToken string, opts ...Option) (*Provider, error) {
	if apiToken == "" {
		return nil, errors.New("hetzner: api token must not be empty")
	}
	p := &Provider{
		apiToken: apiToken,
		client:   hcloud.NewClient(hcloud.WithToken(apiToken)),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// ProvisionServer provisions a new Hetzner Cloud server using the Pulumi
// Automation API (pulumi-hcloud provider). A [pulumiruntime.Runtime] must be
// wired via [WithRuntime] before calling this method.
//
// The Pulumi stack is named "hetzner-<ClusterID>" and is created or updated
// idempotently on every call. The Hetzner Cloud server ID is read from the
// stack's "serverID" output and returned as a decimal string.
//
// Returns [provider.ErrInvalidSpec] when required spec fields are missing.
func (p *Provider) ProvisionServer(ctx context.Context, spec provider.ServerSpec) (string, error) {
	if err := validateSpec(spec); err != nil {
		return "", fmt.Errorf("%w: %s", provider.ErrInvalidSpec, err)
	}
	if p.runtime == nil {
		return "", errors.New("hetzner: Pulumi runtime not configured; wire a Runtime via WithRuntime")
	}
	stackName := "hetzner-" + spec.ClusterID
	res, err := p.runtime.Up(ctx, stackName, serverProgram(spec), pulumiruntime.StackConfig{
		"hcloud:token": p.apiToken,
	})
	if err != nil {
		return "", fmt.Errorf("hetzner: provision cluster %q: %w", spec.ClusterID, err)
	}
	serverID, ok := res.Outputs["serverID"].(string)
	if !ok || serverID == "" {
		return "", errors.New("hetzner: stack output 'serverID' missing or not a string")
	}
	return serverID, nil
}

// DeprovisionServer removes the Hetzner Cloud server identified by serverID
// using the Hetzner Cloud REST API.
//
// serverID must be the decimal string representation of the Hetzner Cloud
// server's numeric ID as returned by [ProvisionServer].
//
// Returns [provider.ErrServerNotFound] when no server with that ID exists.
func (p *Provider) DeprovisionServer(ctx context.Context, serverID string) error {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: invalid server ID format: %w", serverID, err)
	}
	server, _, err := p.client.Server.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, err)
	}
	if server == nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, provider.ErrServerNotFound)
	}
	if _, err := p.client.Server.Delete(ctx, server); err != nil {
		return fmt.Errorf("hetzner: deprovision server %q: %w", serverID, err)
	}
	return nil
}

// GetServer returns the current state of the Hetzner Cloud server identified
// by serverID using the Hetzner Cloud REST API.
//
// serverID must be the decimal string representation of the Hetzner Cloud
// server's numeric ID as returned by [ProvisionServer].
//
// Returns [provider.ErrServerNotFound] when no server with that ID exists.
func (p *Provider) GetServer(ctx context.Context, serverID string) (*provider.ServerInfo, error) {
	id, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("hetzner: get server %q: invalid server ID format: %w", serverID, err)
	}
	server, _, err := p.client.Server.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("hetzner: get server %q: %w", serverID, err)
	}
	if server == nil {
		return nil, fmt.Errorf("hetzner: get server %q: %w", serverID, provider.ErrServerNotFound)
	}
	return serverToInfo(server), nil
}

// ListServers returns all Hetzner Cloud servers visible to the configured API
// token. An empty slice (not nil) is returned when no servers exist.
func (p *Provider) ListServers(ctx context.Context) ([]*provider.ServerInfo, error) {
	servers, err := p.client.Server.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("hetzner: list servers: %w", err)
	}
	infos := make([]*provider.ServerInfo, 0, len(servers))
	for _, s := range servers {
		infos = append(infos, serverToInfo(s))
	}
	return infos, nil
}

// serverToInfo maps a Hetzner Cloud server to the provider-agnostic
// ServerInfo type.
func serverToInfo(s *hcloud.Server) *provider.ServerInfo {
	ipv4 := ""
	if !s.PublicNet.IPv4.IsUnspecified() {
		ipv4 = s.PublicNet.IPv4.IP.String()
	}
	return &provider.ServerInfo{
		ID:         strconv.FormatInt(s.ID, 10),
		Name:       s.Name,
		Status:     string(s.Status),
		PublicIPv4: ipv4,
	}
}

// validateSpec checks that the required ServerSpec fields are non-empty.
func validateSpec(spec provider.ServerSpec) error {
	var errs []error
	if spec.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if spec.ServerType == "" {
		errs = append(errs, errors.New("serverType is required"))
	}
	if spec.Region == "" {
		errs = append(errs, errors.New("region is required"))
	}
	if spec.Image == "" {
		errs = append(errs, errors.New("image is required"))
	}
	if spec.ClusterID == "" {
		errs = append(errs, errors.New("clusterID is required"))
	}
	return errors.Join(errs...)
}

// serverProgram returns the Pulumi inline program that declares the desired
// Hetzner Cloud server state using the pulumi-hcloud provider. Pulumi handles
// idempotency: re-running ProvisionServer for the same ClusterID converges to
// the same cloud state without creating duplicate servers.
func serverProgram(spec provider.ServerSpec) pulumiruntime.ProgramFunc {
	return func(ctx *gopulumi.Context) error {
		server, err := hcloudpulumi.NewServer(ctx, spec.Name, &hcloudpulumi.ServerArgs{
			Name:       gopulumi.String(spec.Name),
			ServerType: gopulumi.String(spec.ServerType),
			Image:      gopulumi.String(spec.Image),
			Location:   gopulumi.String(spec.Region),
			Labels: gopulumi.StringMap{
				"managed-by": gopulumi.String("lx-container-weaver"),
				"cluster-id": gopulumi.String(spec.ClusterID),
			},
		})
		if err != nil {
			return fmt.Errorf("hcloud: declare server resource %q: %w", spec.Name, err)
		}
		// Export the Hetzner Cloud server ID as a string so that callers can
		// use it with DeprovisionServer, GetServer, and ListServers.
		ctx.Export("serverID", server.ID().ApplyT(func(id gopulumi.ID) string {
			return string(id)
		}).(gopulumi.StringOutput))
		ctx.Export("publicIPv4", server.Ipv4Address)
		return nil
	}
}
