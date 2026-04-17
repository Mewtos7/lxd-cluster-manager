// Package orchestrator implements the continuous reconciliation loop described
// in ADR-006. The loop periodically evaluates each cluster's state and drives
// it toward the desired state by making scheduling, scale-out, consolidation,
// and eviction decisions.
package orchestrator

import (
	"context"
	"log/slog"
	"time"
)

// Orchestrator runs the per-cluster reconciliation loop.
type Orchestrator struct {
	interval time.Duration
	logger   *slog.Logger
}

// New creates an Orchestrator that runs a reconciliation pass every interval.
func New(interval time.Duration, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		interval: interval,
		logger:   logger,
	}
}

// Run starts the reconciliation loop. It blocks until ctx is cancelled, which
// triggers a clean exit after the current reconcile pass (if any) completes.
//
// Each iteration calls reconcile to evaluate cluster state. Only one
// scale-out or scale-in action is taken per iteration per cluster to prevent
// oscillation (ADR-006).
func (o *Orchestrator) Run(ctx context.Context) {
	o.logger.Info("orchestrator starting", "interval", o.interval)
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("orchestrator stopped")
			return
		case <-ticker.C:
			o.reconcile(ctx)
		}
	}
}

// reconcile performs a single evaluation pass across all managed clusters.
// This is a stub implementation; the full scheduling, scale-out, and
// consolidation logic will be added in subsequent stories.
func (o *Orchestrator) reconcile(_ context.Context) {
	o.logger.Debug("reconcile pass started")
	// TODO: query cluster list from repository, evaluate each cluster.
	o.logger.Debug("reconcile pass completed")
}
