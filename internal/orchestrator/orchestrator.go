// Package orchestrator implements the continuous reconciliation loop described
// in ADR-006. The loop periodically evaluates each cluster's state and drives
// it toward the desired state by making scheduling, scale-out, consolidation,
// and eviction decisions.
package orchestrator

import (
	"context"
	"log/slog"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"github.com/Mewtos7/lx-container-weaver/internal/provider"
)

// NodeBootstrapRunner is the interface that the Orchestrator uses to execute
// the node bootstrap workflow after a server has been provisioned. It is
// satisfied by [bootstrap.Workflow].
//
// The interface is kept narrow (a single method) so that tests can substitute
// a lightweight stub without pulling in the full bootstrap dependency.
type NodeBootstrapRunner interface {
	// Run executes the node bootstrap workflow for the named node and returns a
	// [bootstrap.Result] that describes whether the node is ready for
	// workload scheduling. Callers must not treat the node as available when
	// [bootstrap.Result.Ready] is false.
	Run(ctx context.Context, nodeName string, cfg bootstrap.Config) bootstrap.Result
}

// NodeInventorySyncer synchronises the node inventory for a single cluster.
// It is satisfied by [inventory.Syncer].
//
// The interface is kept narrow so that tests can substitute a lightweight stub
// without pulling in the full LXD client dependency.
type NodeInventorySyncer interface {
	Sync(ctx context.Context, clusterID string) error
}

// InstanceInventorySyncer synchronises the instance inventory for a single
// cluster. It is satisfied by [inventory.InstanceSyncer].
type InstanceInventorySyncer interface {
	Sync(ctx context.Context, clusterID string) error
}

// Orchestrator runs the per-cluster reconciliation loop.
type Orchestrator struct {
	interval       time.Duration
	logger         *slog.Logger
	provider       provider.HyperscalerProvider
	bootstrap      NodeBootstrapRunner
	clusterRepo    persistence.ClusterRepository
	nodeSyncer     NodeInventorySyncer
	instanceSyncer InstanceInventorySyncer
}

// Option is a functional option for configuring an Orchestrator at
// construction time.
type Option func(*Orchestrator)

// WithProvider wires a [provider.HyperscalerProvider] into the Orchestrator
// so that the reconciliation loop can invoke provisioning and deprovisioning
// operations via the Pulumi Automation API (ADR-005).
//
// If no provider is configured, the reconciliation loop logs a warning and
// skips provisioning steps until a provider is available.
func WithProvider(p provider.HyperscalerProvider) Option {
	return func(o *Orchestrator) { o.provider = p }
}

// WithBootstrapWorkflow wires a [NodeBootstrapRunner] into the Orchestrator.
// The runner is invoked during scale-out after a new server has been
// provisioned via the hyperscaler provider, bridging the gap between cloud
// provisioning and usable LXD cluster capacity (ADR-006).
//
// If no runner is configured, the reconciliation loop skips the bootstrap step
// and logs a warning; newly provisioned nodes will not be added to the cluster
// until a runner is available.
func WithBootstrapWorkflow(r NodeBootstrapRunner) Option {
	return func(o *Orchestrator) { o.bootstrap = r }
}

// WithClusterRepository wires a [persistence.ClusterRepository] into the
// Orchestrator so that the reconciliation loop can enumerate all registered
// clusters on each pass.
//
// If no repository is configured the loop logs a debug message and skips the
// reconciliation step; this allows the service to start without a database
// connection during development.
func WithClusterRepository(r persistence.ClusterRepository) Option {
	return func(o *Orchestrator) { o.clusterRepo = r }
}

// WithNodeSyncer wires a [NodeInventorySyncer] into the Orchestrator. On each
// reconciliation pass the syncer is called for every registered cluster so
// that the node inventory reflects the current LXD cluster-member state.
func WithNodeSyncer(s NodeInventorySyncer) Option {
	return func(o *Orchestrator) { o.nodeSyncer = s }
}

// WithInstanceSyncer wires an [InstanceInventorySyncer] into the Orchestrator.
// On each reconciliation pass the syncer is called for every registered cluster
// so that the instance inventory reflects the current LXD instance state.
func WithInstanceSyncer(s InstanceInventorySyncer) Option {
	return func(o *Orchestrator) { o.instanceSyncer = s }
}

// New creates an Orchestrator that runs a reconciliation pass every interval.
func New(interval time.Duration, logger *slog.Logger, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		interval: interval,
		logger:   logger,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
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

// ReconcileOnce executes a single reconciliation pass synchronously. It is
// equivalent to one tick of the loop driven by [Run] and is primarily
// intended for use in tests and manual validation tooling.
func (o *Orchestrator) ReconcileOnce(ctx context.Context) {
	o.reconcile(ctx)
}

// reconcile performs a single evaluation pass across all managed clusters.
//
// It reads the current cluster list from the repository and calls
// reconcileCluster for each entry. Failures within a single cluster are
// logged and do not abort the pass for the remaining clusters, ensuring the
// loop stays stable across partial failures (ADR-006).
//
// The intended per-cluster scale-out sequence (ADR-006) is:
//  1. Detect that capacity is insufficient (high-water mark exceeded or no
//     node can accept a pending workload).
//  2. Call o.provider.ProvisionServer to create a new cloud server.
//  3. Call o.bootstrap.Run to bootstrap the server as an LXD cluster node.
//     The bootstrap workflow runs precondition / readiness checks before
//     attempting LXD cluster formation; a failed run sets Ready=false and
//     the node status is updated to the error state so that the next
//     reconciliation pass can retry or alert the operator.
//  4. Add the successfully bootstrapped node to the cluster inventory so that
//     the scheduler can place workloads on it.
func (o *Orchestrator) reconcile(ctx context.Context) {
	o.logger.Debug("reconcile pass started")

	if o.clusterRepo == nil {
		o.logger.Debug("reconcile pass skipped: no cluster repository configured")
		return
	}

	clusters, err := o.clusterRepo.ListClusters(ctx)
	if err != nil {
		o.logger.Error("reconcile: failed to list clusters", "error", err)
		return
	}

	o.logger.Debug("reconcile: evaluating clusters", "count", len(clusters))
	for _, cluster := range clusters {
		o.reconcileCluster(ctx, cluster)
	}

	o.logger.Debug("reconcile pass completed", "clusters", len(clusters))
}

// reconcileCluster performs a single reconciliation pass for one cluster.
//
// It runs the node and instance inventory sync steps before any scheduling or
// scaling decisions so that those steps always operate on up-to-date state.
// Errors from individual sync steps are logged and do not abort the remaining
// steps for the same cluster, keeping the loop resilient to partial failures.
func (o *Orchestrator) reconcileCluster(ctx context.Context, cluster *model.Cluster) {
	log := o.logger.With("cluster_id", cluster.ID, "cluster_name", cluster.Name)
	log.Debug("reconcile cluster: started")

	if o.nodeSyncer != nil {
		if err := o.nodeSyncer.Sync(ctx, cluster.ID); err != nil {
			log.Error("reconcile cluster: node inventory sync failed", "error", err)
			// Continue — instance sync and scaling steps are independent.
		}
	}

	if o.instanceSyncer != nil {
		if err := o.instanceSyncer.Sync(ctx, cluster.ID); err != nil {
			log.Error("reconcile cluster: instance inventory sync failed", "error", err)
			// Continue — scaling steps do not depend on a successful instance sync.
		}
	}

	if o.provider == nil {
		log.Debug("reconcile cluster: no provider configured; skipping scaling steps")
	} else {
		if o.bootstrap == nil {
			log.Warn("reconcile cluster: bootstrap workflow not configured; newly provisioned nodes will not be onboarded")
		}
		// TODO: evaluate scaling decisions (ADR-006 high/low-water mark logic)
		// and invoke o.provider.ProvisionServer / o.bootstrap.Run /
		// o.provider.DeprovisionServer as needed.
	}

	log.Debug("reconcile cluster: completed")
}
