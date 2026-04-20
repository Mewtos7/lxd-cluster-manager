package bootstrap_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/memory"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"io"
	"log/slog"
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
