package orchestrator_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/orchestrator"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
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
