# ADR-007 — Live Migration Mechanism

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

Scale-in and workload consolidation (ADR-006) require moving running instances (containers and VMs) from one LXD node to another without stopping them. Live migration must be reliable, observable, and handle failure cases gracefully to avoid data loss or unplanned downtime.

## Decision

Live migration is performed using **LXD's native live migration capability** (`lxc move <instance> <target-node> --live`) invoked via the LXD REST API.

Migration is scoped to **within the same LXD cluster** only. Cross-cluster migration is explicitly out of scope for the initial version.

The migration flow is:

1. Select source node (candidate for scale-in) and identify all instances on it.
2. For each instance, verify the target node has sufficient capacity.
3. Issue the `move` operation via the LXD API.
4. Poll the LXD API until the operation completes or times out.
5. On success: update the manager's database to reflect the new placement.
6. On failure: log the error, mark the migration as failed, abort scale-in for that node, and retain the instance on the source node. Alert via logs/metrics.

## Rationale

- **LXD's built-in live migration** handles the complexity of memory transfer, disk sync (CRIU for containers, QEMU for VMs), and state consistency. There is no benefit to implementing this at the manager level.
- Using the **LXD REST API** (rather than the `lxc` CLI binary) keeps the manager self-contained without a runtime dependency on the CLI tool.
- **Polling for completion** is preferred over callbacks for simplicity and compatibility with LXD's async operation model.

## Alternatives Considered

- **Cold migration (stop, move, start):** Simpler and more reliable, but introduces downtime — unacceptable for production workloads.
- **Cross-cluster migration:** LXD does not natively support cross-cluster live migration; it would require exporting and re-importing instances, which is too disruptive for a live system. Deferred to a future ADR if required.

## Consequences

- Live migration requires LXD nodes to meet prerequisites: shared storage (e.g. Ceph) or `lxd.migration.stateful` support must be available.
- Containers intended for live migration must be started with `--stateful` support enabled.
- The manager must track in-progress migrations to avoid scheduling decisions that conflict with an ongoing move.
- Migration timeouts and retry policies are configurable.
- If live migration fails on a node targeted for scale-in, that node is excluded from scale-in for the current reconciliation cycle and retried in a future cycle.

## Related ADRs

- ADR-006 — Orchestration and Scheduling Strategy
- ADR-008 — Multi-Cluster Management Model
