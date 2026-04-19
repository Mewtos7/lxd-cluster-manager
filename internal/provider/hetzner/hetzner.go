// Package hetzner provides the Hetzner Cloud implementation of the
// HyperscalerProvider interface (ADR-005). Server lifecycle is managed via the
// Pulumi Automation API embedded in the manager binary.
//
// The provider uses [pulumiruntime.Runtime] to drive Pulumi stacks in-process.
// Wire a runtime at construction time via [WithRuntime]. Without a runtime the
// provisioning operations return an error directing the operator to configure
// one.
//
// The full pulumi-hcloud stack program is implemented in the Hetzner Provider
// Adapter story. This package wires the Automation API integration layer.
package hetzner

import (
	"context"
	"errors"
	"fmt"

	gopulumi "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	pulumiruntime "github.com/Mewtos7/lx-container-weaver/internal/pulumi"
)

// Provider is the Hetzner Cloud implementation of provider.HyperscalerProvider.
type Provider struct {
	// apiToken is the Hetzner Cloud API token used by the Pulumi Hetzner
	// provider for authentication.
	apiToken string

	// runtime drives Pulumi stack lifecycle for provisioning operations.
	// Nil when the provider is used without a wired runtime.
	runtime *pulumiruntime.Runtime
}

// Option is a functional option for configuring a [Provider] at construction.
type Option func(*Provider)

// WithRuntime wires a [pulumiruntime.Runtime] into the Provider so that
// [Provider.ProvisionServer] and [Provider.DeprovisionServer] manage cloud
// resources in-process via the Pulumi Automation API (ADR-005).
//
// If this option is not provided, provisioning operations return an error
// directing the operator to configure a runtime.
func WithRuntime(rt *pulumiruntime.Runtime) Option {
	return func(p *Provider) { p.runtime = rt }
}

// New creates a new Hetzner Provider authenticated with the given API token.
// Use [WithRuntime] to attach a Pulumi Automation runtime for provisioning.
func New(apiToken string, opts ...Option) (*Provider, error) {
	if apiToken == "" {
		return nil, errors.New("hetzner: api token must not be empty")
	}
	p := &Provider{apiToken: apiToken}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
}

// ProvisionServer provisions a new Hetzner Cloud server using the Pulumi
// Automation API. A [pulumiruntime.Runtime] must be wired via [WithRuntime]
// before calling this method.
//
// The Pulumi stack is named "hetzner-<ClusterID>" and is created or updated
// idempotently on every call. The server ID is read from the stack's
// "serverID" output.
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

// DeprovisionServer removes the Hetzner Cloud server identified by serverID.
// TODO: implement full stack destroy using Pulumi Automation API once
// the per-cluster stack state model is available (ADR-005 follow-up).
func (p *Provider) DeprovisionServer(_ context.Context, _ string) error {
	return errors.New("hetzner: DeprovisionServer not yet implemented")
}

// GetServer returns the current state of the Hetzner Cloud server identified
// by serverID.
// TODO: implement using Hetzner Cloud API.
func (p *Provider) GetServer(_ context.Context, _ string) (*provider.ServerInfo, error) {
	return nil, errors.New("hetzner: GetServer not yet implemented")
}

// ListServers returns all servers managed by the Hetzner Cloud provider.
// TODO: implement using Hetzner Cloud API.
func (p *Provider) ListServers(_ context.Context) ([]*provider.ServerInfo, error) {
	return nil, errors.New("hetzner: ListServers not yet implemented")
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
// Hetzner Cloud server state. This is a stub that exports a deterministic
// server ID without creating real cloud resources.
//
// The full hcloud provider integration — registering a hcloud.Server resource
// and exporting its real ID — is added in the Hetzner Provider Adapter story.
func serverProgram(spec provider.ServerSpec) pulumiruntime.ProgramFunc {
	return func(ctx *gopulumi.Context) error {
		// Stub: export a deterministic ID derived from the spec until the
		// full pulumi-hcloud provider integration is in place.
		ctx.Export("serverID", gopulumi.String("stub-"+spec.Name))
		return nil
	}
}
