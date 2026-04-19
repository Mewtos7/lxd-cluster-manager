// Package orchestrator implements the continuous reconciliation loop described
// in ADR-006. The loop periodically evaluates each cluster's state and drives
// it toward the desired state by making scheduling, scale-out, consolidation,
// and eviction decisions.
package orchestrator

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Mewtos7/lx-container-weaver/internal/bootstrap"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
	"github.com/Mewtos7/lx-container-weaver/internal/provider"
	"github.com/Mewtos7/lx-container-weaver/internal/scheduler"
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
	nodeRepo       persistence.NodeRepository
	instanceRepo   persistence.InstanceRepository
	nodeSyncer     NodeInventorySyncer
	instanceSyncer InstanceInventorySyncer
	scheduler      scheduler.Scheduler
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

// WithNodeRepository wires a [persistence.NodeRepository] into the
// Orchestrator. The repository is used by the scheduling step to read the
// current node state for placement decisions.
//
// If no repository is configured, the scheduling step is skipped.
func WithNodeRepository(r persistence.NodeRepository) Option {
	return func(o *Orchestrator) { o.nodeRepo = r }
}

// WithInstanceRepository wires a [persistence.InstanceRepository] into the
// Orchestrator. The repository is used by the scheduling step to read current
// instance placements and to record new placement decisions.
//
// If no repository is configured, the scheduling step is skipped.
func WithInstanceRepository(r persistence.InstanceRepository) Option {
	return func(o *Orchestrator) { o.instanceRepo = r }
}

// WithScheduler wires a [scheduler.Scheduler] into the Orchestrator. On each
// reconciliation pass the scheduler is called to assign unplaced instances to
// nodes using the configured placement strategy (ADR-006).
//
// If no scheduler is configured, the scheduling step is skipped and instances
// with no assigned node are left unplaced until a scheduler is available.
func WithScheduler(s scheduler.Scheduler) Option {
	return func(o *Orchestrator) { o.scheduler = s }
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

	scaleOutRequired := o.scheduleCluster(ctx, cluster, log)

	if o.provider == nil {
		log.Debug("reconcile cluster: no provider configured; skipping scaling steps")
	} else {
		if scaleOutRequired {
			o.executeScaleOut(ctx, cluster, log)
		}
	}

	log.Debug("reconcile cluster: completed")
}

// scheduleCluster runs the placement step for one cluster. It lists all nodes
// and instances, finds instances that have not yet been placed on a node
// (NodeID == ""), and calls the scheduler to assign each one.
//
// A successful placement is recorded by updating the instance's NodeID in the
// repository. When no eligible node is available the scheduler returns
// [scheduler.ErrNoCapacity] and the event is logged so that an operator or a
// future scale-out step can react.
//
// scheduleCluster returns true when at least one instance could not be placed
// due to insufficient capacity, signalling that a scale-out event is required.
//
// scheduleCluster is a no-op when the scheduler, node repository, or instance
// repository is not configured.
func (o *Orchestrator) scheduleCluster(ctx context.Context, cluster *model.Cluster, log *slog.Logger) bool {
	if o.scheduler == nil || o.nodeRepo == nil || o.instanceRepo == nil {
		return false
	}

	nodes, err := o.nodeRepo.ListNodes(ctx, cluster.ID)
	if err != nil {
		log.Error("schedule cluster: failed to list nodes", "error", err)
		return false
	}

	instances, err := o.instanceRepo.ListInstances(ctx, cluster.ID)
	if err != nil {
		log.Error("schedule cluster: failed to list instances", "error", err)
		return false
	}

	scaleOutRequired := false
	for _, inst := range instances {
		if inst.NodeID != "" {
			// Instance already placed; nothing to do.
			continue
		}

		req := scheduler.Request{
			CPULimit:    inst.CPULimit,
			MemoryLimit: inst.MemoryLimit,
			DiskLimit:   inst.DiskLimit,
		}

		result, schedErr := o.scheduler.Schedule(nodes, instances, req)
		if errors.Is(schedErr, scheduler.ErrNoCapacity) {
			scaleOutRequired = true
			log.Warn("schedule cluster: no eligible node for instance; scale-out required",
				"instance_id", inst.ID, "instance_name", inst.Name)
			continue
		}
		if schedErr != nil {
			log.Error("schedule cluster: scheduler error", "instance_id", inst.ID, "error", schedErr)
			continue
		}

		// Record the placement decision.
		placed := *inst
		placed.NodeID = result.Node.ID
		if _, updateErr := o.instanceRepo.UpdateInstance(ctx, &placed); updateErr != nil {
			log.Error("schedule cluster: failed to record placement",
				"instance_id", inst.ID, "node_id", result.Node.ID, "error", updateErr)
			continue
		}

		log.Info("schedule cluster: placed instance",
			"instance_id", inst.ID, "instance_name", inst.Name,
			"node_id", result.Node.ID)

		// Refresh the local instances slice so subsequent scheduling decisions
		// in this pass account for the resources committed to this placement.
		for i, in := range instances {
			if in.ID == inst.ID {
				instances[i] = &placed
				break
			}
		}
	}
	return scaleOutRequired
}

// executeScaleOut provisions a new cloud server and bootstraps it as an LXD
// cluster node when placement demand cannot be satisfied by existing capacity.
//
// The sequence follows ADR-006:
//  1. Guard against duplicate provisioning: if any node for the cluster is
//     already in the provisioning state the scale-out is skipped until the
//     previous attempt completes or fails, preventing uncontrolled duplication.
//  2. Build a [provider.ServerSpec] from the cluster's HyperscalerConfig
//     (keys: "server_type", "region", "image"). An incomplete config causes an
//     early exit with a logged error — no cloud resources are touched.
//  3. Call [provider.HyperscalerProvider.ProvisionServer]. On failure the
//     error is logged and the method returns without creating any node record,
//     leaving state unambiguous.
//  4. Create a node record in the [model.NodeStatusProvisioning] state so that
//     subsequent reconciliation passes are aware of the in-flight provisioning.
//     Failure to write the record is logged but does not abort the bootstrap step.
//  5. If a [NodeBootstrapRunner] is configured, run the bootstrap workflow and
//     update the node record to [model.NodeStatusOnline] on success or
//     [model.NodeStatusError] on failure. When no runner is configured, a
//     warning is logged and the node remains in the provisioning state for a
//     subsequent pass to handle.
//
// executeScaleOut is a no-op when no provider is configured.
func (o *Orchestrator) executeScaleOut(ctx context.Context, cluster *model.Cluster, log *slog.Logger) {
	// ── Step 1: anti-duplication guard ──────────────────────────────────────

	if o.nodeRepo != nil {
		nodes, listErr := o.nodeRepo.ListNodes(ctx, cluster.ID)
		if listErr != nil {
			log.Error("scale-out: failed to list nodes for duplicate check", "error", listErr)
			return
		}
		for _, n := range nodes {
			if n.Status == model.NodeStatusProvisioning {
				log.Info("scale-out: skipping; a node is already being provisioned",
					"existing_node_id", n.ID, "existing_node_name", n.Name)
				return
			}
		}
	}

	// ── Step 2: build server spec ────────────────────────────────────────────

	nodeName, nameErr := generateNodeName(cluster.Name)
	if nameErr != nil {
		log.Error("scale-out: failed to generate node name", "error", nameErr)
		return
	}

	spec, specErr := serverSpecFromCluster(cluster, nodeName)
	if specErr != nil {
		log.Error("scale-out: invalid cluster configuration; cannot provision server",
			"cluster_id", cluster.ID, "error", specErr)
		return
	}

	// ── Step 3: provision the server ─────────────────────────────────────────

	serverID, provErr := o.provider.ProvisionServer(ctx, spec)
	if provErr != nil {
		log.Error("scale-out: provisioning failed",
			"cluster_id", cluster.ID, "server_name", spec.Name, "error", provErr)
		return
	}
	log.Info("scale-out: server provisioned",
		"cluster_id", cluster.ID, "server_id", serverID, "server_name", spec.Name)

	// ── Step 4: record node in provisioning state ────────────────────────────

	var nodeRecord *model.Node
	if o.nodeRepo != nil {
		var createErr error
		nodeRecord, createErr = o.nodeRepo.CreateNode(ctx, &model.Node{
			ClusterID:           cluster.ID,
			Name:                nodeName,
			HyperscalerServerID: serverID,
			Status:              model.NodeStatusProvisioning,
		})
		if createErr != nil {
			log.Error("scale-out: failed to record provisioning node",
				"cluster_id", cluster.ID, "server_id", serverID, "error", createErr)
			// Non-fatal: the server was provisioned but we could not persist the
			// record. Log and continue so the bootstrap step is still attempted.
		}
	}

	// ── Step 5: bootstrap and update node status ─────────────────────────────

	if o.bootstrap == nil {
		log.Warn("scale-out: bootstrap workflow not configured; node remains in provisioning state",
			"cluster_id", cluster.ID, "server_id", serverID, "node_name", nodeName)
		return
	}

	bCfg := bootstrapConfigFromCluster(cluster, nodeName)
	result := o.bootstrap.Run(ctx, nodeName, bCfg)

	if nodeRecord == nil || o.nodeRepo == nil {
		// No node record to update; just log the bootstrap outcome.
		if result.Ready {
			log.Info("scale-out: node bootstrapped and online",
				"cluster_id", cluster.ID, "node_name", nodeName)
		} else {
			log.Error("scale-out: bootstrap failed",
				"cluster_id", cluster.ID, "node_name", nodeName,
				"failed_step", result.FailedStep, "error", result.Err)
		}
		return
	}

	updated := *nodeRecord
	if result.Ready {
		updated.Status = model.NodeStatusOnline
		if _, updateErr := o.nodeRepo.UpdateNode(ctx, &updated); updateErr != nil {
			log.Error("scale-out: failed to update node status to online",
				"node_id", nodeRecord.ID, "error", updateErr)
		} else {
			log.Info("scale-out: node bootstrapped and online",
				"cluster_id", cluster.ID, "node_id", nodeRecord.ID, "node_name", nodeName)
		}
	} else {
		updated.Status = model.NodeStatusError
		if _, updateErr := o.nodeRepo.UpdateNode(ctx, &updated); updateErr != nil {
			log.Error("scale-out: failed to update node status to error",
				"node_id", nodeRecord.ID, "error", updateErr)
		}
		log.Error("scale-out: bootstrap failed; node marked as error",
			"cluster_id", cluster.ID, "node_id", nodeRecord.ID, "node_name", nodeName,
			"failed_step", result.FailedStep, "error", result.Err)
	}
}

// generateNodeName returns a unique node name derived from the cluster name and
// a random 4-byte hex suffix (e.g. "prod-cluster-scale-a3f2b1c9").
func generateNodeName(clusterName string) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate node name: %w", err)
	}
	return fmt.Sprintf("%s-scale-%x", clusterName, b), nil
}

// configStringFromMap extracts a string value from a map[string]any. It
// returns ("", false) when the map is nil, the key is absent, or the value is
// not a string.
func configStringFromMap(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// serverSpecFromCluster builds a [provider.ServerSpec] from the cluster's
// HyperscalerConfig map. The following string keys are required:
//   - "server_type": provider-specific server size (e.g. "cx21")
//   - "region":      provider-specific datacenter / AZ (e.g. "fsn1")
//   - "image":       OS image name (e.g. "ubuntu-22.04")
//
// Returns an error when any required field is absent or empty.
func serverSpecFromCluster(cluster *model.Cluster, nodeName string) (provider.ServerSpec, error) {
	get := func(key string) string {
		s, _ := configStringFromMap(cluster.HyperscalerConfig, key)
		return s
	}

	spec := provider.ServerSpec{
		Name:       nodeName,
		ServerType: get("server_type"),
		Region:     get("region"),
		Image:      get("image"),
		ClusterID:  cluster.ID,
	}

	var missing []string
	if spec.ServerType == "" {
		missing = append(missing, "server_type")
	}
	if spec.Region == "" {
		missing = append(missing, "region")
	}
	if spec.Image == "" {
		missing = append(missing, "image")
	}
	if len(missing) > 0 {
		return provider.ServerSpec{}, fmt.Errorf(
			"cluster hyperscaler_config is missing required scale-out fields: %v", missing)
	}
	return spec, nil
}

// bootstrapConfigFromCluster builds a [bootstrap.Config] from the cluster's
// ScalingConfig and HyperscalerConfig maps. newNodeName is used as the joiner
// node name so that the bootstrap workflow targets the newly provisioned server.
//
// Keys read from ScalingConfig:
//   - "bootstrap_trust_token": shared secret for LXD cluster formation
//   - "storage_driver":        LXD storage backend (e.g. "dir", "zfs")
//   - "storage_pool":          storage pool name (e.g. "default")
//   - "seed_node_name":        LXD member name of the existing seed node
//   - "seed_node_address":     listen address of the seed node (host:port)
//
// Keys read from HyperscalerConfig:
//   - "listen_address": the address the new node will advertise (host:port)
//
// Missing keys are left as empty strings; the bootstrap workflow will surface
// failures if required fields are absent.
func bootstrapConfigFromCluster(cluster *model.Cluster, newNodeName string) bootstrap.Config {
	sc := func(key string) string {
		s, _ := configStringFromMap(cluster.ScalingConfig, key)
		return s
	}
	hc := func(key string) string {
		s, _ := configStringFromMap(cluster.HyperscalerConfig, key)
		return s
	}

	return bootstrap.Config{
		ClusterName:   cluster.Name,
		TrustToken:    sc("bootstrap_trust_token"),
		StorageDriver: sc("storage_driver"),
		StoragePool:   sc("storage_pool"),
		SeedNode: bootstrap.NodeConfig{
			Name:          sc("seed_node_name"),
			ListenAddress: sc("seed_node_address"),
		},
		JoinerNode: bootstrap.NodeConfig{
			Name:          newNodeName,
			ListenAddress: hc("listen_address"),
		},
	}
}
