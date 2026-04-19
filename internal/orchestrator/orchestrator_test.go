package orchestrator_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/orchestrator"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	"github.com/Mewtos7/lx-container-weaver/internal/scheduler"
)

// discard is a no-op logger used in tests to suppress output.
var discard = slog.New(slog.NewTextHandler(nopWriter{}, nil))

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// ─── stubs ────────────────────────────────────────────────────────────────────

// stubSyncer records calls to Sync so tests can assert which clusters were
// reconciled and in what order. An optional syncErr is returned on every call.
type stubSyncer struct {
	calls   []string // clusterIDs passed to Sync, in call order
	syncErr error    // error returned from each Sync call (nil → success)
}

func (s *stubSyncer) Sync(_ context.Context, clusterID string) error {
	s.calls = append(s.calls, clusterID)
	return s.syncErr
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// seedCluster creates a cluster in the store and returns it.
func seedCluster(t *testing.T, store *memory.ClusterStore, c *model.Cluster) *model.Cluster {
	t.Helper()
	created, err := store.CreateCluster(context.Background(), c)
	if err != nil {
		t.Fatalf("seedCluster: %v", err)
	}
	return created
}

// newTestOrch returns an Orchestrator with a 1-second interval and the
// provided options, using the discard logger.
func newTestOrch(opts ...orchestrator.Option) *orchestrator.Orchestrator {
	return orchestrator.New(time.Second, discard, opts...)
}

// ─── reconcile: no cluster repository ────────────────────────────────────────

// TestReconcile_NoRepository verifies that the reconciliation pass completes
// without error when no cluster repository is configured.
func TestReconcile_NoRepository(t *testing.T) {
	nodeSyncer := &stubSyncer{}
	instanceSyncer := &stubSyncer{}

	orch := newTestOrch(
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	// Run one reconcile pass directly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orch.ReconcileOnce(ctx)

	if len(nodeSyncer.calls) != 0 {
		t.Errorf("node syncer: want 0 calls when no repo configured, got %d", len(nodeSyncer.calls))
	}
	if len(instanceSyncer.calls) != 0 {
		t.Errorf("instance syncer: want 0 calls when no repo configured, got %d", len(instanceSyncer.calls))
	}
}

// ─── reconcile: empty cluster list ───────────────────────────────────────────

// TestReconcile_EmptyClusterList verifies that the reconciliation pass
// completes without calling any syncer when no clusters are registered.
func TestReconcile_EmptyClusterList(t *testing.T) {
	store := memory.NewClusterStore()
	nodeSyncer := &stubSyncer{}
	instanceSyncer := &stubSyncer{}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(store),
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	orch.ReconcileOnce(context.Background())

	if len(nodeSyncer.calls) != 0 {
		t.Errorf("node syncer: want 0 calls for empty cluster list, got %d", len(nodeSyncer.calls))
	}
	if len(instanceSyncer.calls) != 0 {
		t.Errorf("instance syncer: want 0 calls for empty cluster list, got %d", len(instanceSyncer.calls))
	}
}

// ─── reconcile: normal path ───────────────────────────────────────────────────

// TestReconcile_CallsSyncersForEachCluster verifies that for a known set of
// registered clusters both the node and instance syncer are called once per
// cluster.
func TestReconcile_CallsSyncersForEachCluster(t *testing.T) {
	store := memory.NewClusterStore()
	c1 := seedCluster(t, store, &model.Cluster{Name: "cluster-1"})
	c2 := seedCluster(t, store, &model.Cluster{Name: "cluster-2"})

	nodeSyncer := &stubSyncer{}
	instanceSyncer := &stubSyncer{}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(store),
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	orch.ReconcileOnce(context.Background())

	// Each syncer should be called exactly once per cluster.
	if len(nodeSyncer.calls) != 2 {
		t.Errorf("node syncer: want 2 calls, got %d", len(nodeSyncer.calls))
	}
	if len(instanceSyncer.calls) != 2 {
		t.Errorf("instance syncer: want 2 calls, got %d", len(instanceSyncer.calls))
	}

	// Both cluster IDs must appear in the call records.
	wantIDs := map[string]struct{}{c1.ID: {}, c2.ID: {}}
	for _, id := range nodeSyncer.calls {
		if _, ok := wantIDs[id]; !ok {
			t.Errorf("node syncer: unexpected cluster ID %q", id)
		}
	}
	for _, id := range instanceSyncer.calls {
		if _, ok := wantIDs[id]; !ok {
			t.Errorf("instance syncer: unexpected cluster ID %q", id)
		}
	}
}

// ─── reconcile: failure isolation ────────────────────────────────────────────

// TestReconcile_NodeSyncFailureDoesNotAbortLoop verifies that a failure in the
// node sync step for one cluster does not prevent the instance sync step or the
// reconciliation of subsequent clusters.
func TestReconcile_NodeSyncFailureDoesNotAbortLoop(t *testing.T) {
	store := memory.NewClusterStore()
	seedCluster(t, store, &model.Cluster{Name: "cluster-1"})
	seedCluster(t, store, &model.Cluster{Name: "cluster-2"})

	errSyncer := errors.New("node sync error")
	nodeSyncer := &stubSyncer{syncErr: errSyncer}
	instanceSyncer := &stubSyncer{}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(store),
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	// Must not panic or exit early — just log and continue.
	orch.ReconcileOnce(context.Background())

	// Node syncer still called for both clusters (errors logged, not propagated).
	if len(nodeSyncer.calls) != 2 {
		t.Errorf("node syncer: want 2 calls despite errors, got %d", len(nodeSyncer.calls))
	}
	// Instance syncer must still run for both clusters.
	if len(instanceSyncer.calls) != 2 {
		t.Errorf("instance syncer: want 2 calls even when node sync fails, got %d", len(instanceSyncer.calls))
	}
}

// TestReconcile_InstanceSyncFailureDoesNotAbortLoop verifies that a failure in
// the instance sync step does not prevent the reconciliation of subsequent
// clusters.
func TestReconcile_InstanceSyncFailureDoesNotAbortLoop(t *testing.T) {
	store := memory.NewClusterStore()
	seedCluster(t, store, &model.Cluster{Name: "cluster-1"})
	seedCluster(t, store, &model.Cluster{Name: "cluster-2"})

	nodeSyncer := &stubSyncer{}
	instanceSyncer := &stubSyncer{syncErr: errors.New("instance sync error")}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(store),
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	orch.ReconcileOnce(context.Background())

	if len(nodeSyncer.calls) != 2 {
		t.Errorf("node syncer: want 2 calls, got %d", len(nodeSyncer.calls))
	}
	if len(instanceSyncer.calls) != 2 {
		t.Errorf("instance syncer: want 2 calls despite errors, got %d", len(instanceSyncer.calls))
	}
}

// ─── reconcile: idempotency / no churn ───────────────────────────────────────

// TestReconcile_Idempotent verifies that repeated reconciliation passes with
// unchanged state produce the same number of syncer calls each time and do not
// accumulate side effects.
func TestReconcile_Idempotent(t *testing.T) {
	store := memory.NewClusterStore()
	c := seedCluster(t, store, &model.Cluster{Name: "cluster-1"})

	nodeSyncer := &stubSyncer{}
	instanceSyncer := &stubSyncer{}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(store),
		orchestrator.WithNodeSyncer(nodeSyncer),
		orchestrator.WithInstanceSyncer(instanceSyncer),
	)

	const passes = 3
	for i := range passes {
		orch.ReconcileOnce(context.Background())
		wantCalls := i + 1
		if len(nodeSyncer.calls) != wantCalls {
			t.Errorf("pass %d: node syncer: want %d cumulative calls, got %d",
				i+1, wantCalls, len(nodeSyncer.calls))
		}
		if len(instanceSyncer.calls) != wantCalls {
			t.Errorf("pass %d: instance syncer: want %d cumulative calls, got %d",
				i+1, wantCalls, len(instanceSyncer.calls))
		}
		// Cluster ID must be consistent across all passes.
		if got := nodeSyncer.calls[i]; got != c.ID {
			t.Errorf("pass %d: node syncer: want cluster ID %q, got %q", i+1, c.ID, got)
		}
	}
}

// ─── reconcile: optional syncers ─────────────────────────────────────────────

// TestReconcile_NilSyncersAreSkipped verifies that the reconcile pass
// completes without panicking when no node or instance syncer is configured.
func TestReconcile_NilSyncersAreSkipped(t *testing.T) {
	store := memory.NewClusterStore()
	seedCluster(t, store, &model.Cluster{Name: "cluster-1"})

	// Only a cluster repo — no syncers.
	orch := newTestOrch(orchestrator.WithClusterRepository(store))

	// Must not panic.
	orch.ReconcileOnce(context.Background())
}

// ─── reconcile: scheduler integration ────────────────────────────────────────

const (
	schedGiB = 1024 * 1024 * 1024
)

// seedNode creates a node record in the node store and returns it.
func seedNodeInStore(t *testing.T, store *memory.NodeStore, n *model.Node) *model.Node {
	t.Helper()
	created, err := store.CreateNode(context.Background(), n)
	if err != nil {
		t.Fatalf("seedNodeInStore: %v", err)
	}
	return created
}

// seedInstance creates an instance record in the instance store and returns it.
func seedInstance(t *testing.T, store *memory.InstanceStore, i *model.Instance) *model.Instance {
	t.Helper()
	created, err := store.CreateInstance(context.Background(), i)
	if err != nil {
		t.Fatalf("seedInstance: %v", err)
	}
	return created
}

// TestReconcile_SchedulerPlacesUnplacedInstance verifies that the reconcile
// pass calls the scheduler for each instance without a NodeID and updates
// the instance record with the selected node.
func TestReconcile_SchedulerPlacesUnplacedInstance(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{Name: "cluster-1"})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// Seed one online node with headroom.
	node := seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID:   cluster.ID,
		Name:        "node-1",
		Status:      model.NodeStatusOnline,
		CPUCores:    4,
		MemoryBytes: 8 * schedGiB,
		DiskBytes:   100 * schedGiB,
	})

	// Seed one unplaced instance (NodeID == "").
	inst := seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "inst-1",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   10 * schedGiB,
	})
	if inst.NodeID != "" {
		t.Fatalf("pre-condition: instance should have no NodeID, got %q", inst.NodeID)
	}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
	)

	orch.ReconcileOnce(context.Background())

	// The instance should now be placed on node-1.
	updated, err := instanceStore.GetInstance(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("GetInstance after reconcile: %v", err)
	}
	if updated.NodeID != node.ID {
		t.Errorf("instance NodeID: want %q, got %q", node.ID, updated.NodeID)
	}
}

// TestReconcile_SchedulerNoCapacityIsLogged verifies that when no node has
// sufficient headroom the reconcile pass completes without error and the
// instance remains unplaced (ScaleOutRequired was signalled internally).
func TestReconcile_SchedulerNoCapacityIsLogged(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{Name: "cluster-1"})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// Seed a fully packed node.
	node := seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID:   cluster.ID,
		Name:        "node-1",
		Status:      model.NodeStatusOnline,
		CPUCores:    2,
		MemoryBytes: 4 * schedGiB,
		DiskBytes:   50 * schedGiB,
	})

	// Seed an instance that fills the node completely (using the real node ID),
	// and an unplaced instance that cannot fit.
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "existing",
		NodeID:      node.ID,
		CPULimit:    2,
		MemoryLimit: 4 * schedGiB,
		DiskLimit:   50 * schedGiB,
	})
	unplaced := seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 1 * schedGiB,
		DiskLimit:   10 * schedGiB,
	})

	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
	)

	// Must not panic — ErrNoCapacity is logged, not propagated.
	orch.ReconcileOnce(context.Background())

	// The unplaced instance must remain unplaced.
	got, err := instanceStore.GetInstance(context.Background(), unplaced.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("unplaced instance should remain unplaced, got NodeID %q", got.NodeID)
	}
}

// TestReconcile_SchedulerSkippedWhenNotConfigured verifies that the scheduling
// step is a no-op when no scheduler is wired into the Orchestrator, leaving
// existing instance state unchanged.
func TestReconcile_SchedulerSkippedWhenNotConfigured(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{Name: "cluster-1"})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID:   cluster.ID,
		Name:        "node-1",
		Status:      model.NodeStatusOnline,
		CPUCores:    4,
		MemoryBytes: 8 * schedGiB,
		DiskBytes:   100 * schedGiB,
	})
	unplaced := seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "inst-1",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   10 * schedGiB,
	})

	// No WithScheduler option → scheduling step must be skipped.
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
	)

	orch.ReconcileOnce(context.Background())

	got, err := instanceStore.GetInstance(context.Background(), unplaced.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("instance should not be placed when no scheduler configured, got NodeID %q", got.NodeID)
	}
}

// ─── scale-out stubs ──────────────────────────────────────────────────────────

// stubProvider is a test double for provider.HyperscalerProvider. It records
// calls to ProvisionServer and returns a configurable server ID or error.
type stubProvider struct {
	provisionCalls int
	provisionErr   error
	provisionedID  string // returned on success; defaults to "test-server-id"
}

func (p *stubProvider) ProvisionServer(_ context.Context, _ provider.ServerSpec) (string, error) {
	p.provisionCalls++
	if p.provisionErr != nil {
		return "", p.provisionErr
	}
	id := p.provisionedID
	if id == "" {
		id = "test-server-id"
	}
	return id, nil
}
func (p *stubProvider) DeprovisionServer(_ context.Context, _ string) error { return nil }
func (p *stubProvider) GetServer(_ context.Context, _ string) (*provider.ServerInfo, error) {
	return nil, nil
}
func (p *stubProvider) ListServers(_ context.Context) ([]*provider.ServerInfo, error) {
	return nil, nil
}

// stubBootstrap is a test double for orchestrator.NodeBootstrapRunner. It
// records calls to Run and returns a configurable bootstrap.Result.
type stubBootstrap struct {
	runCalls int
	result   bootstrap.Result
}

func (b *stubBootstrap) Run(_ context.Context, nodeName string, _ bootstrap.Config) bootstrap.Result {
	b.runCalls++
	r := b.result
	r.NodeName = nodeName
	return r
}

// hyperscalerConfig returns a HyperscalerConfig map with the minimum fields
// required by serverSpecFromCluster so that scale-out tests do not need to
// repeat boilerplate.
func hyperscalerConfig() map[string]any {
	return map[string]any{
		"server_type": "cx21",
		"region":      "fsn1",
		"image":       "ubuntu-22.04",
	}
}

// ─── scale-out tests ──────────────────────────────────────────────────────────

// TestScaleOut_TriggeredWhenNoCapacity verifies that when the scheduler cannot
// place an instance (ErrNoCapacity) and a provider is configured, the
// reconcile pass calls ProvisionServer exactly once and creates a node record
// in the provisioning state.
func TestScaleOut_TriggeredWhenNoCapacity(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// Seed a fully packed node so the scheduler returns ErrNoCapacity.
	node := seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID:   cluster.ID,
		Name:        "node-1",
		Status:      model.NodeStatusOnline,
		CPUCores:    1,
		MemoryBytes: 1 * schedGiB,
		DiskBytes:   10 * schedGiB,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "existing",
		NodeID:      node.ID,
		CPULimit:    1,
		MemoryLimit: 1 * schedGiB,
		DiskLimit:   10 * schedGiB,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
	)

	orch.ReconcileOnce(context.Background())

	if prov.provisionCalls != 1 {
		t.Errorf("ProvisionServer: want 1 call, got %d", prov.provisionCalls)
	}

	// A new node record must appear in the provisioning state.
	nodes, err := nodeStore.ListNodes(context.Background(), cluster.ID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	var provisioningNodes []*model.Node
	for _, n := range nodes {
		if n.Status == model.NodeStatusProvisioning {
			provisioningNodes = append(provisioningNodes, n)
		}
	}
	if len(provisioningNodes) != 1 {
		t.Errorf("want 1 provisioning node, got %d", len(provisioningNodes))
	}
	if len(provisioningNodes) > 0 && provisioningNodes[0].HyperscalerServerID != "test-server-id" {
		t.Errorf("provisioning node HyperscalerServerID: want %q, got %q",
			"test-server-id", provisioningNodes[0].HyperscalerServerID)
	}
}

// TestScaleOut_AntiDuplication verifies that when a node is already in the
// provisioning state, a subsequent reconcile pass does not trigger another
// ProvisionServer call, preventing uncontrolled duplicate provisioning.
func TestScaleOut_AntiDuplication(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// Pre-existing provisioning node — simulates a previous scale-out that
	// has not completed yet.
	seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID: cluster.ID,
		Name:      "cluster-1-scale-existing",
		Status:    model.NodeStatusProvisioning,
	})

	// Unplaced instance that would normally trigger scale-out.
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
	)

	orch.ReconcileOnce(context.Background())

	if prov.provisionCalls != 0 {
		t.Errorf("ProvisionServer: want 0 calls (anti-dup), got %d", prov.provisionCalls)
	}
}

// TestScaleOut_BootstrapSuccessUpdatesNodeOnline verifies that a successful
// bootstrap transitions the newly provisioned node to the online state so that
// subsequent reconciliation passes can schedule workloads on it.
func TestScaleOut_BootstrapSuccessUpdatesNodeOnline(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// No online nodes → ErrNoCapacity for any instance.
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	boot := &stubBootstrap{result: bootstrap.Result{Ready: true}}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
		orchestrator.WithBootstrapWorkflow(boot),
	)

	orch.ReconcileOnce(context.Background())

	if prov.provisionCalls != 1 {
		t.Errorf("ProvisionServer: want 1 call, got %d", prov.provisionCalls)
	}
	if boot.runCalls != 1 {
		t.Errorf("bootstrap.Run: want 1 call, got %d", boot.runCalls)
	}

	nodes, err := nodeStore.ListNodes(context.Background(), cluster.ID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Status != model.NodeStatusOnline {
		t.Errorf("node status: want %q, got %q", model.NodeStatusOnline, nodes[0].Status)
	}
}

// TestScaleOut_BootstrapFailureUpdatesNodeError verifies that when the
// bootstrap workflow fails, the node record is updated to the error state so
// that operators can identify and investigate the failed node without leaving
// the system in ambiguous state.
func TestScaleOut_BootstrapFailureUpdatesNodeError(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	boot := &stubBootstrap{result: bootstrap.Result{
		Ready:      false,
		FailedStep: "bootstrap",
		Err:        errors.New("bootstrap failed"),
	}}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
		orchestrator.WithBootstrapWorkflow(boot),
	)

	orch.ReconcileOnce(context.Background())

	if boot.runCalls != 1 {
		t.Errorf("bootstrap.Run: want 1 call, got %d", boot.runCalls)
	}

	nodes, err := nodeStore.ListNodes(context.Background(), cluster.ID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Status != model.NodeStatusError {
		t.Errorf("node status: want %q, got %q", model.NodeStatusError, nodes[0].Status)
	}
}

// TestScaleOut_ProvisioningFailureIsLogged verifies that when ProvisionServer
// returns an error the reconcile pass completes without panic, no node record
// is created, and the unplaced instance is left untouched.
func TestScaleOut_ProvisioningFailureIsLogged(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	unplaced := seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{provisionErr: errors.New("hetzner: rate limited")}
	boot := &stubBootstrap{}

	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
		orchestrator.WithBootstrapWorkflow(boot),
	)

	// Must not panic.
	orch.ReconcileOnce(context.Background())

	if prov.provisionCalls != 1 {
		t.Errorf("ProvisionServer: want 1 call, got %d", prov.provisionCalls)
	}
	if boot.runCalls != 0 {
		t.Errorf("bootstrap.Run: want 0 calls after provisioning failure, got %d", boot.runCalls)
	}

	// No node records should be created.
	nodes, err := nodeStore.ListNodes(context.Background(), cluster.ID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("want 0 node records after provisioning failure, got %d", len(nodes))
	}

	// The unplaced instance must remain unplaced.
	got, err := instanceStore.GetInstance(context.Background(), unplaced.ID)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("unplaced instance should remain unplaced, got NodeID %q", got.NodeID)
	}
}

// TestScaleOut_NoProviderSkipsScaleOut verifies that when no provider is
// configured the reconcile pass completes without calling any provisioning
// operation, even when placement demand cannot be satisfied.
func TestScaleOut_NoProviderSkipsScaleOut(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	// No WithProvider → scale-out must be skipped.
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
	)

	// Must not panic.
	orch.ReconcileOnce(context.Background())

	// No provisioning node should appear.
	nodes, err := nodeStore.ListNodes(context.Background(), cluster.ID)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("want 0 node records when no provider configured, got %d", len(nodes))
	}
}

// TestScaleOut_RepeatedLoopDoesNotDuplicateProvision verifies that running
// several reconcile passes with unchanged state (one provisioning node already
// in flight) does not trigger additional ProvisionServer calls.
func TestScaleOut_RepeatedLoopDoesNotDuplicateProvision(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name:              "cluster-1",
		HyperscalerConfig: hyperscalerConfig(),
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	// Simulate an in-flight provisioning from a previous pass.
	seedNodeInStore(t, nodeStore, &model.Node{
		ClusterID: cluster.ID,
		Name:      "cluster-1-scale-existing",
		Status:    model.NodeStatusProvisioning,
	})
	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
	)

	const passes = 5
	for i := range passes {
		orch.ReconcileOnce(context.Background())
		if prov.provisionCalls != 0 {
			t.Errorf("pass %d: ProvisionServer called %d times; want 0 (anti-dup)",
				i+1, prov.provisionCalls)
		}
	}
}

// TestScaleOut_InvalidClusterConfigSkipsProvision verifies that when the
// cluster's HyperscalerConfig is missing required scale-out fields the
// reconcile pass logs an error and does not call ProvisionServer.
func TestScaleOut_InvalidClusterConfigSkipsProvision(t *testing.T) {
	clusterStore := memory.NewClusterStore()
	cluster := seedCluster(t, clusterStore, &model.Cluster{
		Name: "cluster-1",
		// HyperscalerConfig intentionally empty → missing server_type/region/image.
	})

	nodeStore := memory.NewNodeStore()
	instanceStore := memory.NewInstanceStore()

	seedInstance(t, instanceStore, &model.Instance{
		ClusterID:   cluster.ID,
		Name:        "unplaced",
		CPULimit:    1,
		MemoryLimit: 512 * 1024 * 1024,
		DiskLimit:   5 * schedGiB,
	})

	prov := &stubProvider{}
	orch := newTestOrch(
		orchestrator.WithClusterRepository(clusterStore),
		orchestrator.WithNodeRepository(nodeStore),
		orchestrator.WithInstanceRepository(instanceStore),
		orchestrator.WithScheduler(scheduler.New()),
		orchestrator.WithProvider(prov),
	)

	// Must not panic.
	orch.ReconcileOnce(context.Background())

	if prov.provisionCalls != 0 {
		t.Errorf("ProvisionServer: want 0 calls for invalid config, got %d", prov.provisionCalls)
	}
}
