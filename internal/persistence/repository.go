// Package persistence defines the data access boundary for the manager.
// All database interactions are encapsulated behind interfaces so that the
// business logic remains decoupled from the underlying storage implementation.
//
// The concrete PostgreSQL implementation (using the pgx driver as specified in
// ADR-004) will be added in a dedicated data access story. Tests that exercise
// orchestration and API logic may use in-memory or mock implementations of
// these interfaces.
package persistence

import (
	"context"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// ClusterRepository defines the data access operations for clusters.
type ClusterRepository interface {
	// ListClusters returns all registered clusters.
	ListClusters(ctx context.Context) ([]*model.Cluster, error)

	// GetCluster returns the cluster with the given ID.
	GetCluster(ctx context.Context, id string) (*model.Cluster, error)

	// CreateCluster persists a new cluster record.
	CreateCluster(ctx context.Context, c *model.Cluster) (*model.Cluster, error)

	// UpdateCluster updates a cluster record.
	UpdateCluster(ctx context.Context, c *model.Cluster) (*model.Cluster, error)

	// DeleteCluster removes the cluster with the given ID.
	DeleteCluster(ctx context.Context, id string) error
}

// NodeRepository defines the data access operations for nodes within a cluster.
type NodeRepository interface {
	// ListNodes returns all nodes belonging to the given cluster.
	ListNodes(ctx context.Context, clusterID string) ([]*model.Node, error)

	// GetNode returns the node with the given ID.
	GetNode(ctx context.Context, id string) (*model.Node, error)

	// CreateNode persists a new node record.
	CreateNode(ctx context.Context, n *model.Node) (*model.Node, error)

	// UpdateNode updates a node record.
	UpdateNode(ctx context.Context, n *model.Node) (*model.Node, error)

	// DeleteNode removes the node with the given ID.
	DeleteNode(ctx context.Context, id string) error
}

// BootstrapLocker is a distributed lock that prevents multiple manager
// instances from running the bootstrap coordinator simultaneously.
//
// A single Guard instance uses TryLock and Unlock from one goroutine only;
// implementations do not need to support concurrent callers sharing one
// BootstrapLocker instance.
type BootstrapLocker interface {
	// TryLock attempts to acquire the distributed lock without blocking.
	// It returns (true, nil) when the lock is acquired, (false, nil) when
	// another holder already owns the lock, and (false, err) on I/O failures.
	TryLock(ctx context.Context) (bool, error)

	// Unlock releases a previously acquired lock. It is a no-op when the lock
	// is not held by this instance.
	Unlock(ctx context.Context) error
}

// InstanceRepository defines the data access operations for container/VM
// instances within a cluster.
type InstanceRepository interface {
	// ListInstances returns all instances belonging to the given cluster.
	ListInstances(ctx context.Context, clusterID string) ([]*model.Instance, error)

	// GetInstance returns the instance with the given ID.
	GetInstance(ctx context.Context, id string) (*model.Instance, error)

	// CreateInstance persists a new instance record.
	CreateInstance(ctx context.Context, i *model.Instance) (*model.Instance, error)

	// UpdateInstance updates an instance record.
	UpdateInstance(ctx context.Context, i *model.Instance) (*model.Instance, error)

	// DeleteInstance removes the instance with the given ID.
	DeleteInstance(ctx context.Context, id string) error
}
