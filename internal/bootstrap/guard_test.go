package bootstrap_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// discardLogger returns a slog.Logger that silently discards all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// countingCoordinator records how many times RunBootstrap was called.
type countingCoordinator struct {
	calls int
	err   error
}

func (c *countingCoordinator) RunBootstrap(_ context.Context) error {
	c.calls++
	return c.err
}

// TestGuard_Disabled_SkipsBootstrap verifies Scenario 1: when the feature flag
// is disabled the bootstrap coordinator is never called, regardless of the
// cluster state.
func TestGuard_Disabled_SkipsBootstrap(t *testing.T) {
	repo := memory.NewClusterStore()
	coord := &countingCoordinator{}
	guard := bootstrap.NewGuard(repo, discardLogger())

	if err := guard.Run(context.Background(), false, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 0 {
		t.Errorf("coordinator calls: want 0 (flag disabled), got %d", coord.calls)
	}
}

// TestGuard_Enabled_NoClusters_InvokesCoordinator verifies Scenario 2: when
// the feature flag is enabled and no clusters exist the bootstrap coordinator
// is invoked exactly once.
func TestGuard_Enabled_NoClusters_InvokesCoordinator(t *testing.T) {
	repo := memory.NewClusterStore() // empty store — no clusters
	coord := &countingCoordinator{}
	guard := bootstrap.NewGuard(repo, discardLogger())

	if err := guard.Run(context.Background(), true, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 1 {
		t.Errorf("coordinator calls: want 1 (enabled + no clusters), got %d", coord.calls)
	}
}

// TestGuard_Enabled_ClusterExists_SkipsBootstrap verifies Scenario 3: when
// the feature flag is enabled but at least one cluster already exists the
// bootstrap coordinator is not called.
func TestGuard_Enabled_ClusterExists_SkipsBootstrap(t *testing.T) {
	repo := memory.NewClusterStore()
	_, err := repo.CreateCluster(context.Background(), &model.Cluster{
		Name:                "existing",
		LXDEndpoint:         "https://10.0.0.1:8443",
		HyperscalerProvider: "hetzner",
		HyperscalerConfig:   map[string]any{},
		ScalingConfig:       map[string]any{},
		Status:              "active",
	})
	if err != nil {
		t.Fatalf("setup CreateCluster: %v", err)
	}

	coord := &countingCoordinator{}
	guard := bootstrap.NewGuard(repo, discardLogger())

	if err := guard.Run(context.Background(), true, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 0 {
		t.Errorf("coordinator calls: want 0 (cluster exists), got %d", coord.calls)
	}
}

// TestGuard_Enabled_NoClusters_CoordinatorError propagates errors returned by
// the coordinator.
func TestGuard_Enabled_NoClusters_CoordinatorError(t *testing.T) {
	repo := memory.NewClusterStore()
	want := errors.New("coordinator failure")
	coord := &countingCoordinator{err: want}
	guard := bootstrap.NewGuard(repo, discardLogger())

	err := guard.Run(context.Background(), true, coord)
	if !errors.Is(err, want) {
		t.Errorf("Run error: want %v, got %v", want, err)
	}
	if coord.calls != 1 {
		t.Errorf("coordinator calls: want 1, got %d", coord.calls)
	}
}

// TestGuard_BootstrapCoordinatorFunc verifies that the BootstrapCoordinatorFunc
// adapter correctly implements BootstrapCoordinator.
func TestGuard_BootstrapCoordinatorFunc(t *testing.T) {
	called := false
	fn := bootstrap.BootstrapCoordinatorFunc(func(_ context.Context) error {
		called = true
		return nil
	})

	repo := memory.NewClusterStore()
	guard := bootstrap.NewGuard(repo, discardLogger())

	if err := guard.Run(context.Background(), true, fn); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !called {
		t.Error("BootstrapCoordinatorFunc: want called=true, got false")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Distributed lock tests
// ──────────────────────────────────────────────────────────────────────────────

// TestGuard_WithLocker_AcquiresLock verifies that when a locker is configured
// the guard acquires the lock before invoking the coordinator and releases it
// on return.
func TestGuard_WithLocker_AcquiresLock(t *testing.T) {
	repo := memory.NewClusterStore()
	lock := memory.NewLock()
	coord := &countingCoordinator{}
	guard := bootstrap.NewGuard(repo, discardLogger(), bootstrap.WithLocker(lock))

	if err := guard.Run(context.Background(), true, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 1 {
		t.Errorf("coordinator calls: want 1, got %d", coord.calls)
	}

	// Lock should be released after Run returns.
	acquired, err := lock.TryLock(context.Background())
	if err != nil {
		t.Fatalf("TryLock after Run: %v", err)
	}
	if !acquired {
		t.Error("lock should be free after Run returns, but TryLock returned false")
	}
	// Clean up.
	_ = lock.Unlock(context.Background())
}

// TestGuard_WithLocker_LockAlreadyHeld verifies Scenario 1 from the issue:
// when the distributed lock is already held by another instance the guard
// skips bootstrap and does not invoke the coordinator.
func TestGuard_WithLocker_LockAlreadyHeld(t *testing.T) {
	repo := memory.NewClusterStore()
	lock := memory.NewLock()

	// Simulate another instance holding the lock.
	if _, err := lock.TryLock(context.Background()); err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}

	coord := &countingCoordinator{}
	guard := bootstrap.NewGuard(repo, discardLogger(), bootstrap.WithLocker(lock))

	if err := guard.Run(context.Background(), true, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 0 {
		t.Errorf("coordinator calls: want 0 (lock held by another), got %d", coord.calls)
	}
}

// TestGuard_ConcurrentStartup_ExactlyOneBootstrap verifies Scenario 1 from the
// issue: given two manager instances starting simultaneously, exactly one
// acquires the distributed lock and invokes the bootstrap coordinator while
// the other skips.
//
// The coordinator creates a cluster record to simulate real provisioning. This
// is the guard condition that prevents a second instance from bootstrapping
// even if it acquires the lock after the first instance releases it: the
// cluster check (performed under the lock) will then find an existing cluster
// and skip.
func TestGuard_ConcurrentStartup_ExactlyOneBootstrap(t *testing.T) {
	sharedRepo := memory.NewClusterStore()
	sharedLock := memory.NewLock()

	var (
		mu    sync.Mutex
		boots int
	)

	const instances = 2
	var wg sync.WaitGroup
	wg.Add(instances)

	for i := 0; i < instances; i++ {
		go func() {
			defer wg.Done()
			coord := bootstrap.BootstrapCoordinatorFunc(func(ctx context.Context) error {
				mu.Lock()
				boots++
				mu.Unlock()
				// Simulate real provisioning: create the cluster record so that
				// any subsequent instance sees it during its cluster check.
				_, _ = sharedRepo.CreateCluster(ctx, &model.Cluster{
					Name:                "test-cluster",
					LXDEndpoint:         "https://10.0.0.1:8443",
					HyperscalerProvider: "hetzner",
					HyperscalerConfig:   map[string]any{},
					ScalingConfig:       map[string]any{},
					Status:              "active",
				})
				return nil
			})
			guard := bootstrap.NewGuard(sharedRepo, discardLogger(), bootstrap.WithLocker(sharedLock))
			if err := guard.Run(context.Background(), true, coord); err != nil {
				t.Errorf("Run: unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if boots != 1 {
		t.Errorf("bootstrap invocations: want exactly 1, got %d", boots)
	}
}

// TestGuard_WithLocker_NoLocker_BackwardCompatible verifies that the guard
// without a locker behaves identically to the original behaviour (coordinator
// is called once when enabled and no clusters exist).
func TestGuard_WithLocker_NoLocker_BackwardCompatible(t *testing.T) {
	repo := memory.NewClusterStore()
	coord := &countingCoordinator{}
	// No WithLocker option — original behaviour.
	guard := bootstrap.NewGuard(repo, discardLogger())

	if err := guard.Run(context.Background(), true, coord); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if coord.calls != 1 {
		t.Errorf("coordinator calls: want 1, got %d", coord.calls)
	}
}
