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
