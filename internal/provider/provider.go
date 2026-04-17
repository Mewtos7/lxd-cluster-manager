// Package provider defines the HyperscalerProvider abstraction described in
// ADR-005. Each hyperscaler (Hetzner Cloud, AWS, DigitalOcean, …) implements
// this interface. Orchestration logic is written against the interface so that
// adding a new hyperscaler requires only a new implementation, not changes to
// the orchestration layer.
package provider

import "context"

// ServerSpec describes the desired configuration for a server to be
// provisioned on a hyperscaler.
type ServerSpec struct {
	// Name is the desired hostname / display name for the server.
	Name string

	// ServerType is the provider-specific server size (e.g. "cx21" on Hetzner).
	ServerType string

	// Region is the provider-specific datacenter or availability zone.
	Region string

	// Image is the OS image to boot the server with (e.g. "ubuntu-22.04").
	Image string

	// ClusterID is the internal cluster identifier; used for tagging resources.
	ClusterID string
}

// ServerInfo holds the state of a server returned by the hyperscaler.
type ServerInfo struct {
	// ID is the provider-assigned identifier for the server.
	ID string

	// Name is the server's hostname or display name.
	Name string

	// Status is the current lifecycle state of the server.
	Status string

	// PublicIPv4 is the public IPv4 address, if assigned.
	PublicIPv4 string
}

// HyperscalerProvider is the interface that every hyperscaler integration must
// implement. Implementations manage the lifecycle of cloud servers using the
// Pulumi Automation API (ADR-005) so that state tracking and idempotency are
// handled by Pulumi.
type HyperscalerProvider interface {
	// ProvisionServer provisions a new cloud server according to spec and
	// returns the provider-assigned server ID.
	ProvisionServer(ctx context.Context, spec ServerSpec) (serverID string, err error)

	// DeprovisionServer removes the cloud server with the given provider ID.
	DeprovisionServer(ctx context.Context, serverID string) error

	// GetServer returns the current state of the server with the given provider ID.
	GetServer(ctx context.Context, serverID string) (*ServerInfo, error)

	// ListServers returns all servers currently managed by this provider.
	ListServers(ctx context.Context) ([]*ServerInfo, error)
}
