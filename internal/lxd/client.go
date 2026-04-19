// Package lxd defines the LXD client integration layer (ADR-006, ADR-007).
// It provides a [Client] interface that abstracts all LXD REST API calls
// needed by the inventory-sync, live-migration, and orchestration stories.
// Callers import only this interface; concrete implementations are wired at
// construction time, enabling easy substitution with the in-memory fake
// provided by the [fake] sub-package for unit tests.
package lxd

import "context"

// Client is the interface that the manager uses to communicate with an LXD
// cluster. All operations are scoped to a single cluster endpoint, so a new
// Client is created per cluster (using its LXDEndpoint from the database).
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
// # Error semantics
//
// Implementations must wrap errors using fmt.Errorf("…: %w", sentinel) so
// that callers can use errors.Is to distinguish failure kinds:
//
//   - [ErrNodeNotFound] — the requested cluster member does not exist.
//   - [ErrInstanceNotFound] — the requested instance does not exist.
//   - [ErrUnreachable] — the LXD endpoint cannot be contacted.
//   - [ErrMigrationFailed] — a live-migration operation failed.
//
// Any other error is a transient or unexpected failure and may be retried.
type Client interface {
	// GetClusterMembers returns information about all members in the LXD
	// cluster. An empty slice (not nil) is returned when the cluster has no
	// members.
	GetClusterMembers(ctx context.Context) ([]NodeInfo, error)

	// GetClusterMember returns the current state of the named cluster member.
	//
	// Returns [ErrNodeNotFound] if no member with that name exists.
	GetClusterMember(ctx context.Context, name string) (*NodeInfo, error)

	// GetNodeResources returns resource capacity information for the named
	// cluster member, including CPU core count, total and used memory, and
	// total and used disk space.
	//
	// Returns [ErrNodeNotFound] if no member with that name exists.
	GetNodeResources(ctx context.Context, nodeName string) (*NodeResources, error)

	// ListInstances returns all instances (containers and VMs) managed by the
	// LXD cluster. An empty slice (not nil) is returned when no instances
	// exist.
	ListInstances(ctx context.Context) ([]InstanceInfo, error)

	// GetInstance returns the current state of the named instance.
	//
	// Returns [ErrInstanceNotFound] if no instance with that name exists.
	GetInstance(ctx context.Context, name string) (*InstanceInfo, error)

	// MoveInstance live-migrates the named instance to the specified target
	// cluster member following the LXD live-migration protocol (ADR-007).
	// The method blocks until the operation completes or ctx is cancelled.
	//
	// Returns [ErrInstanceNotFound] if the source instance does not exist.
	// Returns [ErrNodeNotFound] if the target cluster member does not exist.
	// Returns [ErrMigrationFailed] if the migration operation fails.
	MoveInstance(ctx context.Context, instanceName, targetNode string) error

	// GetClusterStatus returns the current cluster formation state of the LXD
	// node. Callers use this to determine whether a node is already part of a
	// cluster before attempting InitCluster or JoinCluster.
	GetClusterStatus(ctx context.Context) (*ClusterStatus, error)

	// GetClusterCertificate returns the PEM-encoded TLS certificate of the LXD
	// server. Joining nodes pass this certificate to JoinCluster so they can
	// verify the seed's identity during the handshake.
	GetClusterCertificate(ctx context.Context) (string, error)

	// InitCluster initialises a new LXD cluster on the seed node using the
	// provided preseed configuration (listen address, trust token, storage
	// pool). The operation is idempotent: if the node is already clustered it
	// returns [ErrClusterAlreadyBootstrapped] and the caller must treat that
	// as a successful no-op.
	InitCluster(ctx context.Context, cfg ClusterInitConfig) error

	// JoinCluster adds this node to an existing LXD cluster identified by the
	// seed address and certificate in cfg. The operation is idempotent: if the
	// node is already clustered it returns [ErrClusterAlreadyBootstrapped].
	JoinCluster(ctx context.Context, cfg ClusterJoinConfig) error
}
