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

// GuardOption is a functional option for Guard.
type GuardOption func(*Guard)

// WithLocker configures a distributed lock on the Guard. When set, Run will
// attempt to acquire the lock before invoking the bootstrap coordinator; if
// the lock is already held by another instance the bootstrap step is skipped.
func WithLocker(l persistence.BootstrapLocker) GuardOption {
	return func(g *Guard) {
		g.locker = l
	}
}

// Guard is the top-level trigger gate for initial cluster bootstrap. It
// inspects the feature flag and the cluster repository at startup, invoking
// the BootstrapCoordinator only when bootstrap should proceed. The guard is
// deterministic and side-effect free when the feature flag is disabled.
type Guard struct {
	repo   persistence.ClusterRepository
	logger *slog.Logger
	locker persistence.BootstrapLocker // optional; nil means no distributed locking
}

// NewGuard returns a Guard that uses repo to check whether any clusters already
// exist and logger for structured startup log messages. Optional GuardOptions
// (e.g. WithLocker) may be passed to extend guard behaviour.
func NewGuard(repo persistence.ClusterRepository, logger *slog.Logger, opts ...GuardOption) *Guard {
	g := &Guard{repo: repo, logger: logger}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// Run evaluates the initial bootstrap conditions and invokes
// coordinator.RunBootstrap when all of the following are true:
//   - enabled is true (INITIAL_BOOTSTRAP_ENABLED=true),
//   - the distributed lock (if configured) is successfully acquired, and
//   - the cluster repository contains no clusters (checked under the lock).
//
// Acquiring the lock before the cluster check prevents a time-of-check/
// time-of-use race in concurrent startup: only one instance can hold the lock,
// so the cluster check and subsequent provisioning are effectively serialised.
//
// In all other cases the bootstrap path is skipped and a log message explains
// why. The distributed lock, when held, is released on return regardless of
// whether the coordinator succeeds or fails.
func (g *Guard) Run(ctx context.Context, enabled bool, coordinator BootstrapCoordinator) error {
	if !enabled {
		g.logger.Info("initial bootstrap disabled; skipping",
			"hint", "set INITIAL_BOOTSTRAP_ENABLED=true to enable")
		return nil
	}

	if g.locker != nil {
		acquired, err := g.locker.TryLock(ctx)
		if err != nil {
			return fmt.Errorf("bootstrap guard: acquire lock: %w", err)
		}
		if !acquired {
			g.logger.Info("skipping initial bootstrap: another instance is handling it")
			return nil
		}
		defer func() {
			if err := g.locker.Unlock(context.Background()); err != nil {
				g.logger.Error("bootstrap guard: release lock", "error", err)
			}
		}()
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
