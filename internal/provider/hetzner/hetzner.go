// Package hetzner provides the Hetzner Cloud implementation of the
// HyperscalerProvider interface (ADR-005). Server lifecycle is managed via the
// Pulumi Automation API embedded in the manager binary.
//
// This is a stub implementation. The full Pulumi stack integration and Hetzner
// Cloud provider configuration will be added in the hyperscaler integration
// story.
package hetzner

import (
	"context"
	"errors"

	"github.com/Mewtos7/lx-container-weaver/internal/provider"
)

// Provider is the Hetzner Cloud implementation of provider.HyperscalerProvider.
type Provider struct {
	// apiToken is the Hetzner Cloud API token used by the Pulumi Hetzner
	// provider for authentication.
	apiToken string
}

// New creates a new Hetzner Provider authenticated with the given API token.
func New(apiToken string) (*Provider, error) {
	if apiToken == "" {
		return nil, errors.New("hetzner: api token must not be empty")
	}
	return &Provider{apiToken: apiToken}, nil
}

// ProvisionServer provisions a new Hetzner Cloud server.
// TODO: implement using Pulumi Automation API with pulumi-hcloud provider.
func (p *Provider) ProvisionServer(_ context.Context, _ provider.ServerSpec) (string, error) {
	return "", errors.New("hetzner: ProvisionServer not yet implemented")
}

// DeprovisionServer removes a Hetzner Cloud server.
// TODO: implement using Pulumi Automation API.
func (p *Provider) DeprovisionServer(_ context.Context, _ string) error {
	return errors.New("hetzner: DeprovisionServer not yet implemented")
}

// GetServer returns the current state of a Hetzner Cloud server.
// TODO: implement using Hetzner Cloud API.
func (p *Provider) GetServer(_ context.Context, _ string) (*provider.ServerInfo, error) {
	return nil, errors.New("hetzner: GetServer not yet implemented")
}

// ListServers returns all servers managed by the Hetzner Cloud provider.
// TODO: implement using Hetzner Cloud API.
func (p *Provider) ListServers(_ context.Context) ([]*provider.ServerInfo, error) {
	return nil, errors.New("hetzner: ListServers not yet implemented")
}
