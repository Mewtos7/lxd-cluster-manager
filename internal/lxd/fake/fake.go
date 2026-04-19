// Package fake provides an in-memory implementation of [lxd.Client] for use
// in unit tests of packages that depend on the LXD integration layer (e.g.
// the orchestrator, inventory-sync, and migration stories). It is not intended
// for production use.
//
// Seed the fake with test data before passing it to code under test:
//
//	f := fake.New()
//	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
//	f.AddInstance(lxd.InstanceInfo{Name: "web-01", Location: "lxd1"})
//
//	// Pass f wherever a lxd.Client is expected.
//	orchestrator := orchestrator.New(f, ...)
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
)

// compile-time check: Fake must satisfy lxd.Client.
var _ lxd.Client = (*Fake)(nil)

// MoveRecord records a single MoveInstance call made against the fake.
type MoveRecord struct {
	InstanceName string
	TargetNode   string
}

// Fake is a thread-safe in-memory implementation of [lxd.Client] for testing.
// All write methods (AddNode, AddInstance, etc.) are safe to call concurrently
// with client methods.
type Fake struct {
	mu        sync.RWMutex
	nodes     map[string]lxd.NodeInfo      // keyed by node name
	resources map[string]lxd.NodeResources // keyed by node name
	instances map[string]lxd.InstanceInfo  // keyed by instance name

	// clustered tracks whether this fake node has been initialised as part of
	// a cluster (via InitCluster or JoinCluster, or seeded via SetClustered).
	clustered bool

	// clusterStatus is returned by GetClusterStatus when it has been seeded
	// via SetClusterStatus.
	clusterStatus *lxd.ClusterStatus

	// certificate is returned by GetClusterCertificate. Seed it via
	// SetClusterCertificate.
	certificate string

	// InitError, if non-nil, is returned by InitCluster for every call.
	InitError error

	// JoinError, if non-nil, is returned by JoinCluster for every call.
	JoinError error

	// MoveError, if non-nil, is returned by MoveInstance for every call.
	MoveError error

	// Moves records the history of MoveInstance calls.
	Moves []MoveRecord

	// InitCalls records the ClusterInitConfig passed to each InitCluster call.
	InitCalls []lxd.ClusterInitConfig

	// JoinCalls records the ClusterJoinConfig passed to each JoinCluster call.
	JoinCalls []lxd.ClusterJoinConfig
}

// New returns an empty Fake with no nodes or instances.
func New() *Fake {
	return &Fake{
		nodes:     make(map[string]lxd.NodeInfo),
		resources: make(map[string]lxd.NodeResources),
		instances: make(map[string]lxd.InstanceInfo),
	}
}

// SetClustered seeds the fake's cluster formation state. When true,
// GetClusterStatus reports Enabled: true and InitCluster / JoinCluster return
// [lxd.ErrClusterAlreadyBootstrapped].
func (f *Fake) SetClustered(enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clustered = enabled
}

// SetClusterStatus seeds a specific ClusterStatus to be returned by
// GetClusterStatus. This takes precedence over the simpler SetClustered flag.
func (f *Fake) SetClusterStatus(s lxd.ClusterStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := s
	f.clusterStatus = &cp
	f.clustered = s.Enabled
}

// SetClusterCertificate seeds the PEM certificate returned by
// GetClusterCertificate.
func (f *Fake) SetClusterCertificate(pem string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.certificate = pem
}

// AddNode seeds the fake with a cluster member. Subsequent calls with the same
// node Name overwrite the existing entry.
func (f *Fake) AddNode(n lxd.NodeInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[n.Name] = n
}

// SetNodeResources seeds the fake with resource data for the named node.
func (f *Fake) SetNodeResources(nodeName string, r lxd.NodeResources) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resources[nodeName] = r
}

// AddInstance seeds the fake with an instance. Subsequent calls with the same
// instance Name overwrite the existing entry.
func (f *Fake) AddInstance(i lxd.InstanceInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.instances[i.Name] = i
}

// RemoveNode removes a node from the fake, simulating it going offline.
func (f *Fake) RemoveNode(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.nodes, name)
	delete(f.resources, name)
}

// RemoveInstance removes an instance from the fake.
func (f *Fake) RemoveInstance(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.instances, name)
}

// GetClusterMembers returns all seeded nodes in an unspecified order.
func (f *Fake) GetClusterMembers(_ context.Context) ([]lxd.NodeInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]lxd.NodeInfo, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, n)
	}
	return out, nil
}

// GetClusterMember returns the seeded node with the given name.
// Returns [lxd.ErrNodeNotFound] if no node with that name was seeded.
func (f *Fake) GetClusterMember(_ context.Context, name string) (*lxd.NodeInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	n, ok := f.nodes[name]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get cluster member %q: %w", name, lxd.ErrNodeNotFound)
	}
	cp := n
	return &cp, nil
}

// GetNodeResources returns the seeded resources for the named node.
// Returns [lxd.ErrNodeNotFound] if no resources were seeded for that node.
func (f *Fake) GetNodeResources(_ context.Context, nodeName string) (*lxd.NodeResources, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	r, ok := f.resources[nodeName]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get node resources %q: %w", nodeName, lxd.ErrNodeNotFound)
	}
	cp := r
	return &cp, nil
}

// ListInstances returns all seeded instances in an unspecified order.
func (f *Fake) ListInstances(_ context.Context) ([]lxd.InstanceInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]lxd.InstanceInfo, 0, len(f.instances))
	for _, i := range f.instances {
		out = append(out, i)
	}
	return out, nil
}

// GetInstance returns the seeded instance with the given name.
// Returns [lxd.ErrInstanceNotFound] if no instance with that name was seeded.
func (f *Fake) GetInstance(_ context.Context, name string) (*lxd.InstanceInfo, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	i, ok := f.instances[name]
	if !ok {
		return nil, fmt.Errorf("fake lxd: get instance %q: %w", name, lxd.ErrInstanceNotFound)
	}
	cp := i
	return &cp, nil
}

// MoveInstance records the move in [Fake.Moves]. If [Fake.MoveError] is set,
// that error is returned immediately without updating the instance's Location.
// Otherwise, the instance's Location field is updated to targetNode, simulating
// a successful migration.
//
// Returns [lxd.ErrInstanceNotFound] if the instance is not seeded.
// Returns [lxd.ErrNodeNotFound] if the target node is not seeded.
func (f *Fake) MoveInstance(_ context.Context, instanceName, targetNode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Moves = append(f.Moves, MoveRecord{InstanceName: instanceName, TargetNode: targetNode})

	if f.MoveError != nil {
		return f.MoveError
	}

	inst, ok := f.instances[instanceName]
	if !ok {
		return fmt.Errorf("fake lxd: move instance %q: %w", instanceName, lxd.ErrInstanceNotFound)
	}
	if _, ok := f.nodes[targetNode]; !ok {
		return fmt.Errorf("fake lxd: move instance %q: target %q: %w", instanceName, targetNode, lxd.ErrNodeNotFound)
	}

	inst.Location = targetNode
	f.instances[instanceName] = inst
	return nil
}

// GetClusterStatus returns the seeded cluster formation state. If
// SetClusterStatus was called, that value is returned. Otherwise a status
// derived from the SetClustered flag is returned.
func (f *Fake) GetClusterStatus(_ context.Context) (*lxd.ClusterStatus, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.clusterStatus != nil {
		cp := *f.clusterStatus
		return &cp, nil
	}
	return &lxd.ClusterStatus{Enabled: f.clustered}, nil
}

// GetClusterCertificate returns the certificate seeded via SetClusterCertificate.
// Returns an error if no certificate has been seeded.
func (f *Fake) GetClusterCertificate(_ context.Context) (string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.certificate == "" {
		return "", fmt.Errorf("fake lxd: get cluster certificate: no certificate seeded")
	}
	return f.certificate, nil
}

// InitCluster simulates initialising the seed node. If InitError is set it is
// returned immediately. If the node is already clustered,
// [lxd.ErrClusterAlreadyBootstrapped] is returned. On success the fake marks
// itself as clustered and records the config in InitCalls.
func (f *Fake) InitCluster(_ context.Context, cfg lxd.ClusterInitConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.InitCalls = append(f.InitCalls, cfg)

	if f.InitError != nil {
		return f.InitError
	}
	if f.clustered {
		return fmt.Errorf("fake lxd: init cluster: %w", lxd.ErrClusterAlreadyBootstrapped)
	}
	f.clustered = true
	if f.clusterStatus == nil {
		f.clusterStatus = &lxd.ClusterStatus{}
	}
	f.clusterStatus.Enabled = true
	f.clusterStatus.ServerName = cfg.ServerName
	return nil
}

// JoinCluster simulates adding this node to an existing cluster. If JoinError
// is set it is returned immediately. If the node is already clustered,
// [lxd.ErrClusterAlreadyBootstrapped] is returned. On success the fake marks
// itself as clustered and records the config in JoinCalls.
func (f *Fake) JoinCluster(_ context.Context, cfg lxd.ClusterJoinConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.JoinCalls = append(f.JoinCalls, cfg)

	if f.JoinError != nil {
		return f.JoinError
	}
	if f.clustered {
		return fmt.Errorf("fake lxd: join cluster: %w", lxd.ErrClusterAlreadyBootstrapped)
	}
	f.clustered = true
	if f.clusterStatus == nil {
		f.clusterStatus = &lxd.ClusterStatus{}
	}
	f.clusterStatus.Enabled = true
	f.clusterStatus.ServerName = cfg.ServerName
	return nil
}
