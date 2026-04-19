// Package inventory implements the node inventory sync described in the M2 Core
// Control Plane roadmap milestone. The [Syncer] fetches current cluster member
// state from LXD and reconciles it with the persisted node records so that
// the orchestration and scheduling code always operates on an up-to-date view
// of available infrastructure.
//
// # Sync algorithm
//
// Each [Syncer.Sync] call performs a two-phase operation:
//
//  1. Fetch phase — all cluster member data (status, resources) is read from
//     LXD. If the LXD endpoint is unreachable the method returns an error
//     immediately and the persisted state is left unchanged.
//
//  2. Reconcile phase — the fetched data is applied to the repository:
//     - Members present in LXD but absent from the repository are created.
//     - Members present in both LXD and the repository are updated to reflect
//     the latest status and resource figures.
//     - Members present in the repository but absent from LXD are marked
//     [model.NodeStatusOffline] so that the scheduler does not place
//     workloads on nodes that are no longer reachable.
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

// Syncer reconciles the node inventory in a [persistence.NodeRepository] with
// live cluster member data fetched via a [lxd.Client].
//
// A single Syncer instance is safe for concurrent use by multiple goroutines.
type Syncer struct {
	client lxd.Client
	repo   persistence.NodeRepository
	logger *slog.Logger
}

// New creates a Syncer that uses client to fetch LXD state and repo to persist
// node records. logger receives structured diagnostic messages for each sync
// operation.
func New(client lxd.Client, repo persistence.NodeRepository, logger *slog.Logger) *Syncer {
	return &Syncer{
		client: client,
		repo:   repo,
		logger: logger,
	}
}

// memberData bundles the LXD node info and its associated resource figures for
// a single cluster member. Resources may be nil when the resource endpoint was
// unavailable.
type memberData struct {
	info      lxd.NodeInfo
	resources *lxd.NodeResources
}

// Sync performs a full reconciliation of node inventory for the given cluster.
//
// It fetches all LXD cluster members, loads the current persisted nodes, and
// applies the upsert/offline logic described in the package documentation.
//
// An error is returned when:
//   - The LXD cluster-members endpoint cannot be reached (persisted state is
//     not modified in this case).
//   - A repository read or write operation fails.
//
// Individual resource-endpoint failures are logged at Warn level and do not
// abort the sync; the affected node is upserted with zero resource figures or
// with the previously persisted values.
func (s *Syncer) Sync(ctx context.Context, clusterID string) error {
	// ── Phase 1: Fetch all data from LXD ────────────────────────────────────
	//
	// We collect all LXD data before touching the repository so that a partial
	// LXD failure never leaves the DB in an inconsistent state.

	members, err := s.client.GetClusterMembers(ctx)
	if err != nil {
		return fmt.Errorf("inventory sync: fetch cluster members: %w", err)
	}

	fetched := make([]memberData, 0, len(members))
	for _, m := range members {
		res, resErr := s.client.GetNodeResources(ctx, m.Name)
		if resErr != nil {
			s.logger.Warn("inventory sync: node resources unavailable; using previous values",
				"node", m.Name, "err", resErr)
		}
		fetched = append(fetched, memberData{info: m, resources: res})
	}

	// ── Phase 2: Load current repository state ───────────────────────────────

	existing, err := s.repo.ListNodes(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("inventory sync: list existing nodes: %w", err)
	}

	// Index persisted nodes by their LXD member name for O(1) lookup.
	byLXDName := make(map[string]*model.Node, len(existing))
	for _, n := range existing {
		byLXDName[n.LXDMemberName] = n
	}

	// ── Phase 3: Upsert each LXD member ─────────────────────────────────────

	seenNames := make(map[string]struct{}, len(fetched))
	for _, md := range fetched {
		seenNames[md.info.Name] = struct{}{}
		status := mapLXDStatus(md.info.Status)

		if db, ok := byLXDName[md.info.Name]; ok {
			// Node already tracked — update mutable fields.
			updated := *db
			updated.Status = status
			applyResources(&updated, md.resources)
			if _, err := s.repo.UpdateNode(ctx, &updated); err != nil {
				return fmt.Errorf("inventory sync: update node %q: %w", md.info.Name, err)
			}
			s.logger.Debug("inventory sync: updated node",
				"node", md.info.Name, "status", status)
		} else {
			// New member — create a persisted record.
			n := &model.Node{
				ClusterID:     clusterID,
				Name:          md.info.Name,
				LXDMemberName: md.info.Name,
				Status:        status,
			}
			applyResources(n, md.resources)
			if _, err := s.repo.CreateNode(ctx, n); err != nil {
				return fmt.Errorf("inventory sync: create node %q: %w", md.info.Name, err)
			}
			s.logger.Debug("inventory sync: created node",
				"node", md.info.Name, "status", status)
		}
	}

	// ── Phase 4: Mark removed members offline ────────────────────────────────
	//
	// Nodes that were previously tracked but are no longer returned by LXD
	// have been removed from the cluster. We mark them offline rather than
	// deleting them so that their history and metadata are preserved for
	// operator inspection and future reconciliation decisions.

	for lxdName, node := range byLXDName {
		if _, seen := seenNames[lxdName]; seen {
			continue
		}
		updated := *node
		updated.Status = model.NodeStatusOffline
		if _, err := s.repo.UpdateNode(ctx, &updated); err != nil {
			// A failure here is logged but does not abort the sync: the
			// other nodes have already been reconciled successfully and
			// rolling back would leave the inventory in a worse state.
			s.logger.Warn("inventory sync: failed to mark removed node as offline",
				"node", lxdName, "err", err)
			continue
		}
		s.logger.Debug("inventory sync: marked removed node as offline", "node", lxdName)
	}

	s.logger.Info("inventory sync: completed",
		"cluster_id", clusterID,
		"lxd_members", len(fetched),
		"persisted_nodes", len(existing))
	return nil
}

// mapLXDStatus converts an LXD cluster-member status string into a
// [model.NodeStatus] constant. Unrecognised status values are mapped to
// [model.NodeStatusOffline] to prevent unknown nodes from being treated as
// schedulable capacity.
func mapLXDStatus(lxdStatus string) string {
	switch lxdStatus {
	case "Online":
		return model.NodeStatusOnline
	case "Evacuating":
		return model.NodeStatusDraining
	default:
		return model.NodeStatusOffline
	}
}

// applyResources copies CPU, memory, and disk capacity figures from r into n.
// When r is nil (e.g. the resource endpoint was unavailable) n is left
// unchanged so that the previous values are preserved rather than zeroed out
// on a transient failure.
func applyResources(n *model.Node, r *lxd.NodeResources) {
	if r == nil {
		return
	}
	n.CPUCores = int(r.CPU.Total)
	n.MemoryBytes = int64(r.Memory.Total)
	n.DiskBytes = int64(r.Disk.Total)
}
