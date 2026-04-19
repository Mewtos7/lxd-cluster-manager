// Package inventory implements the node and instance inventory sync described
// in the M2 Core Control Plane roadmap milestone. This file provides the
// [InstanceSyncer] which reconciles container and VM records in a
// [persistence.InstanceRepository] with live instance data fetched via a
// [lxd.Client].
//
// # Sync algorithm
//
// Each [InstanceSyncer.Sync] call performs a five-phase operation:
//
//  1. Fetch phase — all instance data is read from LXD. If the LXD endpoint is
//     unreachable the method returns an error immediately and the persisted
//     state is left unchanged.
//
//  2. Resolve phase — the list of persisted nodes for the cluster is loaded so
//     that each instance's LXD Location field can be translated into the
//     corresponding persisted Node.ID.  Instances whose Location does not match
//     any known node are persisted with an empty NodeID; the scheduler must
//     treat them as unplaceable until a subsequent sync resolves the placement.
//
//  3. Load phase — the current persisted instance records for the cluster are
//     fetched and indexed by name for O(1) lookup during reconciliation.
//
//  4. Upsert phase — each LXD instance is created (if new) or updated (if
//     already tracked) with the latest status, placement (NodeID), and config.
//
//  5. Mark-disappeared phase — instances present in the repository but absent
//     from LXD are marked [model.InstanceStatusUnknown] and their NodeID is
//     cleared so the scheduler does not operate on stale records.
//
// The algorithm is idempotent: repeated invocations with the same LXD state
// converge to the same repository state without creating duplicates.

package inventory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Mewtos7/lx-container-weaver/internal/lxd"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
	"github.com/Mewtos7/lx-container-weaver/internal/persistence/model"
)

// InstanceSyncer reconciles the instance inventory in a
// [persistence.InstanceRepository] with live instance data fetched via a
// [lxd.Client]. It resolves instance placement by correlating the LXD
// Location field with persisted node records from a [persistence.NodeRepository].
//
// A single InstanceSyncer instance is safe for concurrent use by multiple
// goroutines.
type InstanceSyncer struct {
	client    lxd.Client
	instances persistence.InstanceRepository
	nodes     persistence.NodeRepository
	logger    *slog.Logger
}

// NewInstanceSyncer creates an InstanceSyncer that uses client to fetch LXD
// state, instances to persist instance records, and nodes to resolve placement.
// logger receives structured diagnostic messages for each sync operation.
func NewInstanceSyncer(
	client lxd.Client,
	instances persistence.InstanceRepository,
	nodes persistence.NodeRepository,
	logger *slog.Logger,
) *InstanceSyncer {
	return &InstanceSyncer{
		client:    client,
		instances: instances,
		nodes:     nodes,
		logger:    logger,
	}
}

// Sync performs a full reconciliation of instance inventory for the given
// cluster.
//
// It fetches all LXD instances, loads the current persisted instances and
// nodes, and applies the upsert/unknown logic described in the package
// documentation.
//
// An error is returned when:
//   - The LXD instance-list endpoint cannot be reached (persisted state is not
//     modified in this case).
//   - A repository read or write operation fails.
func (s *InstanceSyncer) Sync(ctx context.Context, clusterID string) error {
	// ── Phase 1: Fetch all instance data from LXD ────────────────────────────
	//
	// We collect all LXD data before touching the repository so that a partial
	// LXD failure never leaves the DB in an inconsistent state.

	lxdInstances, err := s.client.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("instance inventory sync: fetch instances: %w", err)
	}

	// ── Phase 2: Resolve placement ────────────────────────────────────────────
	//
	// Build a map from LXD member name → persisted Node.ID so we can set the
	// correct NodeID on each instance record. We load nodes before instances to
	// keep placement resolution close to the fetch phase; neither is modified here.

	persistedNodes, err := s.nodes.ListNodes(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("instance inventory sync: list nodes for placement: %w", err)
	}

	nodeIDByLXDName := make(map[string]string, len(persistedNodes))
	for _, n := range persistedNodes {
		nodeIDByLXDName[n.LXDMemberName] = n.ID
	}

	// ── Phase 3: Load current repository state ───────────────────────────────

	existing, err := s.instances.ListInstances(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("instance inventory sync: list existing instances: %w", err)
	}

	// Index persisted instances by name for O(1) lookup.
	byName := make(map[string]*model.Instance, len(existing))
	for _, i := range existing {
		byName[i.Name] = i
	}

	// ── Phase 4: Upsert each LXD instance ────────────────────────────────────

	seenNames := make(map[string]struct{}, len(lxdInstances))
	for _, info := range lxdInstances {
		seenNames[info.Name] = struct{}{}
		status := mapLXDInstanceStatus(info.Status)
		nodeID := nodeIDByLXDName[info.Location] // empty string if location unknown

		if db, ok := byName[info.Name]; ok {
			// Instance already tracked — update mutable fields.
			updated := *db
			updated.Status = status
			updated.NodeID = nodeID
			updated.Config = convertInstanceConfig(info.Config)
			if _, err := s.instances.UpdateInstance(ctx, &updated); err != nil {
				return fmt.Errorf("instance inventory sync: update instance %q: %w", info.Name, err)
			}
			s.logger.Debug("instance inventory sync: updated instance",
				"instance", info.Name, "status", status, "node_id", nodeID)
		} else {
			// New instance — create a persisted record.
			i := &model.Instance{
				ClusterID:    clusterID,
				NodeID:       nodeID,
				Name:         info.Name,
				InstanceType: info.InstanceType,
				Status:       status,
				Config:       convertInstanceConfig(info.Config),
			}
			if _, err := s.instances.CreateInstance(ctx, i); err != nil {
				return fmt.Errorf("instance inventory sync: create instance %q: %w", info.Name, err)
			}
			s.logger.Debug("instance inventory sync: created instance",
				"instance", info.Name, "status", status, "node_id", nodeID)
		}
	}

	// ── Phase 5: Mark disappeared instances as unknown ────────────────────────
	//
	// Instances that were previously tracked but are no longer returned by LXD
	// may have been deleted or have disappeared from the cluster view. We mark
	// them unknown and clear their NodeID rather than deleting them so that
	// their history is preserved for operator inspection.

	for name, inst := range byName {
		if _, seen := seenNames[name]; seen {
			continue
		}
		updated := *inst
		updated.Status = model.InstanceStatusUnknown
		updated.NodeID = ""
		if _, err := s.instances.UpdateInstance(ctx, &updated); err != nil {
			// Log and continue: other instances have already been reconciled
			// and rolling back would leave the inventory in a worse state.
			s.logger.Warn("instance inventory sync: failed to mark disappeared instance as unknown",
				"instance", name, "err", err)
			continue
		}
		s.logger.Debug("instance inventory sync: marked disappeared instance as unknown",
			"instance", name)
	}

	s.logger.Info("instance inventory sync: completed",
		"cluster_id", clusterID,
		"lxd_instances", len(lxdInstances),
		"persisted_instances", len(existing))
	return nil
}

// mapLXDInstanceStatus converts an LXD instance status string into a
// [model.InstanceStatus] constant. Unrecognised status values are mapped to
// [model.InstanceStatusUnknown] to prevent unexpected states from being treated
// as schedulable.
func mapLXDInstanceStatus(lxdStatus string) string {
	switch lxdStatus {
	case "Running":
		return model.InstanceStatusRunning
	case "Stopped":
		return model.InstanceStatusStopped
	case "Frozen":
		return model.InstanceStatusFrozen
	default:
		return model.InstanceStatusUnknown
	}
}

// convertInstanceConfig converts the LXD instance config map (map[string]string)
// to the persistence model format (map[string]any). Returns nil when c is nil.
func convertInstanceConfig(c map[string]string) map[string]any {
	if c == nil {
		return nil
	}
	out := make(map[string]any, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
