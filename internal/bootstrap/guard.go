package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
)

// BootstrapCoordinator is called by Guard when all conditions for initial
// cluster bootstrap are satisfied (feature flag enabled and no clusters exist).
type BootstrapCoordinator interface {
	RunBootstrap(ctx context.Context) error
}

// BootstrapCoordinatorFunc is a function that implements BootstrapCoordinator.
// It allows plain functions to be used wherever a BootstrapCoordinator is
// expected.
type BootstrapCoordinatorFunc func(ctx context.Context) error

// RunBootstrap implements BootstrapCoordinator.
func (f BootstrapCoordinatorFunc) RunBootstrap(ctx context.Context) error {
	return f(ctx)
}

// Guard is the top-level trigger gate for initial cluster bootstrap. It
// inspects the feature flag and the cluster repository at startup, invoking
// the BootstrapCoordinator only when bootstrap should proceed. The guard is
// deterministic and side-effect free when the feature flag is disabled.
type Guard struct {
	repo   persistence.ClusterRepository
	logger *slog.Logger
}

// NewGuard returns a Guard that uses repo to check whether any clusters already
// exist and logger for structured startup log messages.
func NewGuard(repo persistence.ClusterRepository, logger *slog.Logger) *Guard {
	return &Guard{repo: repo, logger: logger}
}

// Run evaluates the initial bootstrap conditions and invokes
// coordinator.RunBootstrap when all of the following are true:
//   - enabled is true (INITIAL_BOOTSTRAP_ENABLED=true), and
//   - the cluster repository contains no clusters.
//
// In all other cases the bootstrap path is skipped and a log message explains
// why.
func (g *Guard) Run(ctx context.Context, enabled bool, coordinator BootstrapCoordinator) error {
	if !enabled {
		g.logger.Info("initial bootstrap disabled; skipping",
			"hint", "set INITIAL_BOOTSTRAP_ENABLED=true to enable")
		return nil
	}

	clusters, err := g.repo.ListClusters(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap guard: list clusters: %w", err)
	}

	if len(clusters) > 0 {
		g.logger.Info("skipping initial bootstrap: cluster already exists",
			"existing_clusters", len(clusters))
		return nil
	}

	g.logger.Info("no existing clusters found; invoking bootstrap coordinator")

	return coordinator.RunBootstrap(ctx)
}
