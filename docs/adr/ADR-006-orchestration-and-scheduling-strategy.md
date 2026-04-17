# ADR-006 — Orchestration and Scheduling Strategy

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The core value of LX Container Weaver is its ability to automatically manage LXD node capacity in response to workload demand. This requires a well-defined strategy for:

1. Deciding where to place a new instance (container or VM) (scheduling).
2. Detecting when capacity is insufficient and provisioning more (scale-out).
3. Detecting when capacity is over-provisioned and consolidating workloads to free nodes (scale-in).
4. Evicting instances from a node safely before decommissioning it.

## Decision

The orchestrator runs a **continuous reconciliation loop** that periodically evaluates the state of each cluster and drives it toward the desired state. Scheduling, scale-out, consolidation, and eviction are all handled within this loop.

### Placement (scheduling)

New instances (containers and VMs) are placed using a **bin-packing strategy**: select the node with the highest current utilisation that still has sufficient headroom for the workload's requested CPU, memory, and disk. This minimises fragmentation and delays scale-out.

### Scale-out trigger

A scale-out is triggered when:
- A new workload cannot be placed on any existing node, **or**
- Any node's CPU or memory utilisation exceeds a configurable high-water mark (default: 80%) for a sustained period (default: 5 minutes).

On trigger: provision a new server via the hyperscaler provider (ADR-005), bootstrap it as an LXD node, and add it to the cluster.

### Scale-in / consolidation trigger

A scale-in is triggered when:
- One or more nodes' CPU and memory utilisation fall below a configurable low-water mark (default: 20%) for a sustained period (default: 15 minutes), **and**
- The workloads on those nodes can be migrated to the remaining nodes without exceeding the high-water mark.

On trigger: live-migrate all workloads off the candidate node (ADR-007), remove the node from the LXD cluster, and deprovision the server.

### Reconciliation loop

- Loop interval: configurable, default 60 seconds.
- Each iteration reads current node metrics and workload state from LXD and the database.
- Only one scale-out or scale-in action is taken per loop iteration per cluster to avoid oscillation.
- A cooldown period (default: 10 minutes) is enforced after any scale-out or scale-in event before the next can be triggered.

## Rationale

- A reconciliation loop (rather than event-driven triggers) provides a simple, predictable, and debuggable control plane model.
- Bin-packing placement maximises utilisation before triggering scale-out, directly reducing cloud costs.
- Water-mark thresholds with sustained-period requirements prevent thrashing from short-lived spikes.
- Cooldown periods prevent oscillation between scale-out and scale-in states.

## Alternatives Considered

- **Event-driven scheduling (react immediately to workload changes):** Lower latency, but more complex to implement, test, and reason about. Reconciliation loops are a proven pattern (Kubernetes uses the same model).
- **Spread placement (distribute evenly across nodes):** Maximises redundancy but increases the number of nodes required, raising costs — contrary to the project's cost-efficiency goal.

## Consequences

- All thresholds (high-water mark, low-water mark, sustained period, cooldown) are configurable via the manager's configuration file.
- The reconciliation loop must be observable: each iteration logs decisions and reasons, and metrics are exposed for dashboarding.
- The loop must handle partial failures gracefully (e.g. a failed provisioning attempt should not block the next iteration).
- The scheduling algorithm is encapsulated in a replaceable `Scheduler` interface to allow alternative strategies in the future.

## Related ADRs

- ADR-005 — Hyperscaler Integration Approach
- ADR-007 — Live Migration Mechanism
- ADR-008 — Multi-Cluster Management Model
