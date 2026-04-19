package inventory_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/inventory"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/lxd/fake"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// discard is a no-op logger used in tests to suppress output.
var discard = slog.New(slog.NewTextHandler(nopWriter{}, nil))

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

const (
	clusterID = "cluster-1"
	// GiB is one gibibyte in bytes, used to express memory and disk sizes
	// in a readable way throughout the test file.
	GiB = 1024 * 1024 * 1024
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// newSyncer returns a Syncer wired with the provided fake LXD client and a
// fresh in-memory node repository.
func newSyncer(f *fake.Fake) (*inventory.Syncer, *memory.NodeStore) {
	store := memory.NewNodeStore()
	s := inventory.New(f, store, discard)
	return s, store
}

// seedNode creates a node record in the store and returns it.
func seedNode(t *testing.T, store *memory.NodeStore, n *model.Node) *model.Node {
	t.Helper()
	created, err := store.CreateNode(context.Background(), n)
	if err != nil {
		t.Fatalf("seedNode: %v", err)
	}
	return created
}

// listNodes returns all nodes for clusterID from the store.
func listNodes(t *testing.T, store *memory.NodeStore) []*model.Node {
	t.Helper()
	nodes, err := store.ListNodes(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("listNodes: %v", err)
	}
	return nodes
}

// nodeByLXDName finds the node with the given LXDMemberName in ns or returns nil.
func nodeByLXDName(ns []*model.Node, name string) *model.Node {
	for _, n := range ns {
		if n.LXDMemberName == name {
			return n
		}
	}
	return nil
}

// ─── Sync: basic creation ─────────────────────────────────────────────────────

// TestSync_CreatesNewNodes verifies that LXD members that do not yet exist in
// the repository are created during sync.
func TestSync_CreatesNewNodes(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})
	f.SetNodeResources("lxd1", lxd.NodeResources{
		CPU:    lxd.CPUResources{Total: 4},
		Memory: lxd.MemoryResources{Total: 8 * GiB},
		Disk:   lxd.DiskResources{Total: 100 * GiB},
	})
	f.SetNodeResources("lxd2", lxd.NodeResources{
		CPU:    lxd.CPUResources{Total: 8},
		Memory: lxd.MemoryResources{Total: 16 * GiB},
		Disk:   lxd.DiskResources{Total: 200 * GiB},
	})

	syncer, store := newSyncer(f)
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}

	n1 := nodeByLXDName(nodes, "lxd1")
	if n1 == nil {
		t.Fatal("want node lxd1 to be created")
	}
	if n1.Status != model.NodeStatusOnline {
		t.Errorf("lxd1 status: want %q, got %q", model.NodeStatusOnline, n1.Status)
	}
	if n1.CPUCores != 4 {
		t.Errorf("lxd1 CPUCores: want 4, got %d", n1.CPUCores)
	}
	if n1.ClusterID != clusterID {
		t.Errorf("lxd1 ClusterID: want %q, got %q", clusterID, n1.ClusterID)
	}

	n2 := nodeByLXDName(nodes, "lxd2")
	if n2 == nil {
		t.Fatal("want node lxd2 to be created")
	}
	if n2.CPUCores != 8 {
		t.Errorf("lxd2 CPUCores: want 8, got %d", n2.CPUCores)
	}
}

// ─── Sync: status update ──────────────────────────────────────────────────────

// TestSync_UpdatesExistingNodeStatus verifies that when a node already exists
// in the repository, its status is updated to match the LXD-reported value.
func TestSync_UpdatesExistingNodeStatus(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Offline"})
	f.SetNodeResources("lxd1", lxd.NodeResources{})

	syncer, store := newSyncer(f)
	seedNode(t, store, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	nodes := listNodes(t, store)
	n := nodeByLXDName(nodes, "lxd1")
	if n == nil {
		t.Fatal("want node lxd1 to exist after sync")
	}
	if n.Status != model.NodeStatusOffline {
		t.Errorf("lxd1 status: want %q, got %q", model.NodeStatusOffline, n.Status)
	}
}

// TestSync_UpdatesResourceCapacity verifies that resource figures are
// refreshed during an update pass.
func TestSync_UpdatesResourceCapacity(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.SetNodeResources("lxd1", lxd.NodeResources{
		CPU:    lxd.CPUResources{Total: 16},
		Memory: lxd.MemoryResources{Total: 32 * GiB},
		Disk:   lxd.DiskResources{Total: 500 * GiB},
	})

	syncer, store := newSyncer(f)
	seedNode(t, store, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
		CPUCores:      4,
		MemoryBytes:   4 * GiB,
		DiskBytes:     100 * GiB,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	nodes := listNodes(t, store)
	n := nodeByLXDName(nodes, "lxd1")
	if n == nil {
		t.Fatal("want node lxd1 to exist after sync")
	}
	if n.CPUCores != 16 {
		t.Errorf("CPUCores: want 16, got %d", n.CPUCores)
	}
	if n.MemoryBytes != 32*GiB {
		t.Errorf("MemoryBytes: want %d, got %d", 32*GiB, n.MemoryBytes)
	}
}

// ─── Sync: removed node ───────────────────────────────────────────────────────

// TestSync_MarksRemovedNodeOffline verifies that a node present in the
// repository but absent from the current LXD member list is marked offline.
func TestSync_MarksRemovedNodeOffline(t *testing.T) {
	f := fake.New()
	// lxd1 is gone from the cluster; lxd2 is still present.
	f.AddNode(lxd.NodeInfo{Name: "lxd2", Status: "Online"})
	f.SetNodeResources("lxd2", lxd.NodeResources{})

	syncer, store := newSyncer(f)
	seedNode(t, store, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
	})
	seedNode(t, store, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd2",
		LXDMemberName: "lxd2",
		Status:        model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes after sync, got %d", len(nodes))
	}

	n1 := nodeByLXDName(nodes, "lxd1")
	if n1 == nil {
		t.Fatal("want lxd1 to remain in repository (not deleted)")
	}
	if n1.Status != model.NodeStatusOffline {
		t.Errorf("lxd1 status: want %q, got %q", model.NodeStatusOffline, n1.Status)
	}

	n2 := nodeByLXDName(nodes, "lxd2")
	if n2 == nil {
		t.Fatal("want lxd2 to remain in repository")
	}
	if n2.Status != model.NodeStatusOnline {
		t.Errorf("lxd2 status: want %q, got %q", model.NodeStatusOnline, n2.Status)
	}
}

// ─── Sync: idempotency ────────────────────────────────────────────────────────

// TestSync_Idempotent verifies that running Sync twice with the same LXD
// state produces the same repository state without duplicating records.
func TestSync_Idempotent(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.SetNodeResources("lxd1", lxd.NodeResources{
		CPU: lxd.CPUResources{Total: 4},
	})

	syncer, store := newSyncer(f)

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 1 {
		t.Fatalf("want 1 node after two syncs, got %d (duplicate created?)", len(nodes))
	}
	if nodes[0].Status != model.NodeStatusOnline {
		t.Errorf("status: want %q, got %q", model.NodeStatusOnline, nodes[0].Status)
	}
}

// ─── Sync: LXD failure ───────────────────────────────────────────────────────

// TestSync_LXDUnreachable verifies that when GetClusterMembers fails the
// repository is not modified and the error is propagated to the caller.
func TestSync_LXDUnreachable(t *testing.T) {
	unreachable := &errClusterMembersClient{err: lxd.ErrUnreachable}
	store := memory.NewNodeStore()
	syncer := inventory.New(unreachable, store, discard)

	// Seed one node so we can confirm it is not mutated.
	ctx := context.Background()
	seeded, err := store.CreateNode(ctx, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
	})
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}

	syncErr := syncer.Sync(ctx, clusterID)
	if syncErr == nil {
		t.Fatal("Sync: want error when LXD is unreachable, got nil")
	}
	if !errors.Is(syncErr, lxd.ErrUnreachable) {
		t.Errorf("Sync error: want to wrap ErrUnreachable, got %v", syncErr)
	}

	// Persisted state must be unchanged.
	got, err := store.GetNode(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("GetNode after failed sync: %v", err)
	}
	if got.Status != model.NodeStatusOnline {
		t.Errorf("node status: want %q unchanged, got %q", model.NodeStatusOnline, got.Status)
	}
}

// ─── Sync: status mapping ─────────────────────────────────────────────────────

// TestSync_StatusMapping verifies that the LXD status strings are correctly
// translated to the persistence model status constants.
func TestSync_StatusMapping(t *testing.T) {
	tests := []struct {
		lxdStatus  string
		wantStatus string
	}{
		{"Online", model.NodeStatusOnline},
		{"Offline", model.NodeStatusOffline},
		{"Evacuating", model.NodeStatusDraining},
		{"Unknown", model.NodeStatusOffline},
		{"", model.NodeStatusOffline},
	}

	for _, tc := range tests {
		t.Run(tc.lxdStatus, func(t *testing.T) {
			f := fake.New()
			f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: tc.lxdStatus})
			f.SetNodeResources("lxd1", lxd.NodeResources{})

			syncer, store := newSyncer(f)
			if err := syncer.Sync(context.Background(), clusterID); err != nil {
				t.Fatalf("Sync: %v", err)
			}

			nodes := listNodes(t, store)
			if len(nodes) != 1 {
				t.Fatalf("want 1 node, got %d", len(nodes))
			}
			if nodes[0].Status != tc.wantStatus {
				t.Errorf("status: want %q, got %q", tc.wantStatus, nodes[0].Status)
			}
		})
	}
}

// TestSync_EmptyCluster verifies that syncing against an LXD cluster with no
// members is a no-op and does not return an error.
func TestSync_EmptyCluster(t *testing.T) {
	f := fake.New() // no nodes seeded

	syncer, store := newSyncer(f)
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync with empty cluster: unexpected error: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 0 {
		t.Fatalf("want 0 nodes for empty cluster, got %d", len(nodes))
	}
}

// TestSync_ResourcesUnavailable verifies that when GetNodeResources fails for a
// node, the sync still creates/updates the node record without aborting. For a
// newly created node the resource fields remain zero; for an existing node the
// previously persisted values are preserved.
func TestSync_ResourcesUnavailable(t *testing.T) {
	// Use a fake that returns an error for GetNodeResources.
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	// Do NOT call SetNodeResources → GetNodeResources will return ErrNodeNotFound.

	syncer, store := newSyncer(f)
	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: unexpected error when resources unavailable: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 1 {
		t.Fatalf("want 1 node even when resources unavailable, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Status != model.NodeStatusOnline {
		t.Errorf("status: want %q, got %q", model.NodeStatusOnline, n.Status)
	}
	// A newly created node has zero resource fields when the endpoint is
	// unavailable (there are no previous values to preserve).
	if n.CPUCores != 0 || n.MemoryBytes != 0 || n.DiskBytes != 0 {
		t.Errorf("want zero resources for new node when endpoint unavailable, got cpu=%d mem=%d disk=%d",
			n.CPUCores, n.MemoryBytes, n.DiskBytes)
	}
}

// TestSync_ResourcesUnavailablePreservesExisting verifies that when
// GetNodeResources fails for an already-tracked node, the previously persisted
// resource values are kept rather than being zeroed out.
func TestSync_ResourcesUnavailablePreservesExisting(t *testing.T) {
	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	// No resources seeded → GetNodeResources returns ErrNodeNotFound.

	syncer, store := newSyncer(f)

	// Seed the node with known resource values.
	seedNode(t, store, &model.Node{
		ClusterID:     clusterID,
		Name:          "lxd1",
		LXDMemberName: "lxd1",
		Status:        model.NodeStatusOnline,
		CPUCores:      8,
		MemoryBytes:   16 * GiB,
		DiskBytes:     200 * GiB,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	nodes := listNodes(t, store)
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	// Previous resource values must be unchanged when the endpoint is unavailable.
	if n.CPUCores != 8 {
		t.Errorf("CPUCores: want 8 (preserved), got %d", n.CPUCores)
	}
	if n.MemoryBytes != 16*GiB {
		t.Errorf("MemoryBytes: want %d (preserved), got %d", 16*GiB, n.MemoryBytes)
	}
	if n.DiskBytes != 200*GiB {
		t.Errorf("DiskBytes: want %d (preserved), got %d", 200*GiB, n.DiskBytes)
	}
}

// TestSync_MultipleClusterIsolation verifies that nodes belonging to different
// clusters are not mixed during a sync pass.
func TestSync_MultipleClusterIsolation(t *testing.T) {
	const otherCluster = "cluster-other"

	f := fake.New()
	f.AddNode(lxd.NodeInfo{Name: "lxd1", Status: "Online"})
	f.SetNodeResources("lxd1", lxd.NodeResources{})

	syncer, store := newSyncer(f)

	// Seed a node in a different cluster.
	seedNode(t, store, &model.Node{
		ClusterID:     otherCluster,
		Name:          "other-node",
		LXDMemberName: "other-node",
		Status:        model.NodeStatusOnline,
	})

	if err := syncer.Sync(context.Background(), clusterID); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// The synced cluster should have exactly one node.
	clusterNodes, err := store.ListNodes(context.Background(), clusterID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(clusterNodes) != 1 {
		t.Fatalf("cluster %q: want 1 node, got %d", clusterID, len(clusterNodes))
	}

	// The other cluster's node must not be modified.
	otherNodes, err := store.ListNodes(context.Background(), otherCluster)
	if err != nil {
		t.Fatalf("ListNodes other: %v", err)
	}
	if len(otherNodes) != 1 {
		t.Fatalf("other cluster: want 1 node unchanged, got %d", len(otherNodes))
	}
}

// ─── stubs ────────────────────────────────────────────────────────────────────

// errClusterMembersClient is a minimal lxd.Client stub that returns a
// configurable error from GetClusterMembers and panics on any other method.
// It is used to test the LXD-unreachable failure path.
type errClusterMembersClient struct {
	err error
}

func (c *errClusterMembersClient) GetClusterMembers(_ context.Context) ([]lxd.NodeInfo, error) {
	return nil, c.err
}

func (c *errClusterMembersClient) GetClusterMember(_ context.Context, _ string) (*lxd.NodeInfo, error) {
	panic("unexpected call to GetClusterMember")
}

func (c *errClusterMembersClient) GetNodeResources(_ context.Context, _ string) (*lxd.NodeResources, error) {
	panic("unexpected call to GetNodeResources")
}

func (c *errClusterMembersClient) ListInstances(_ context.Context) ([]lxd.InstanceInfo, error) {
	panic("unexpected call to ListInstances")
}

func (c *errClusterMembersClient) GetInstance(_ context.Context, _ string) (*lxd.InstanceInfo, error) {
	panic("unexpected call to GetInstance")
}

func (c *errClusterMembersClient) MoveInstance(_ context.Context, _, _ string) error {
	panic("unexpected call to MoveInstance")
}

func (c *errClusterMembersClient) GetClusterStatus(_ context.Context) (*lxd.ClusterStatus, error) {
	panic("unexpected call to GetClusterStatus")
}

func (c *errClusterMembersClient) GetClusterCertificate(_ context.Context) (string, error) {
	panic("unexpected call to GetClusterCertificate")
}

func (c *errClusterMembersClient) InitCluster(_ context.Context, _ lxd.ClusterInitConfig) error {
	panic("unexpected call to InitCluster")
}

func (c *errClusterMembersClient) JoinCluster(_ context.Context, _ lxd.ClusterJoinConfig) error {
	panic("unexpected call to JoinCluster")
}
