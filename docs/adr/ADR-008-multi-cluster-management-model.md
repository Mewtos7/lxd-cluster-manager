# ADR-008 — Multi-Cluster Management Model

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

LX Container Weaver is designed to manage multiple independent LXD clusters from a single manager instance. Each cluster may have its own set of nodes, instances (containers and VMs), and scaling configuration. The management model must ensure strict isolation between clusters while allowing unified visibility and control through the manager API.

## Decision

Each LXD cluster is represented as an **isolated management domain** within the manager:

- Clusters are registered in the manager's database with their own identity, LXD API endpoint(s), credentials, hyperscaler configuration, and scaling policy.
- All API resources (nodes, instances, scaling events) are scoped to a cluster by a `cluster_id` foreign key.
- The reconciliation loop (ADR-006) runs **independently per cluster** — a failure or scaling event in one cluster does not affect others.
- Each cluster has its own Pulumi stack for hyperscaler provisioning (ADR-005).
- Cross-cluster operations (e.g. moving a workload between clusters) are not supported in the initial version.

### API structure

Cluster-scoped resources follow a nested path pattern:

```
/v1/clusters/{cluster_id}/nodes
/v1/clusters/{cluster_id}/instances
/v1/clusters/{cluster_id}/events
```

Top-level cluster management:

```
GET    /v1/clusters
POST   /v1/clusters
GET    /v1/clusters/{cluster_id}
PUT    /v1/clusters/{cluster_id}
DELETE /v1/clusters/{cluster_id}
```

## Rationale

- **Strict isolation** prevents a runaway scaling event or LXD connectivity issue in one cluster from cascading to others.
- **Per-cluster reconciliation** is simpler to reason about and test than a global scheduler that must coordinate across cluster boundaries.
- **Nested API paths** make the resource ownership model explicit and allow per-cluster access control to be added in the future without restructuring the API.

## Alternatives Considered

- **Separate manager instance per cluster:** Maximum isolation, but operational overhead scales linearly with the number of clusters. Unified visibility and configuration become difficult.
- **Global scheduler across clusters:** Allows cross-cluster placement optimisation, but adds significant complexity and coordination overhead with no identified use case at this stage.

## Consequences

- The initial bootstrap always creates at least one cluster before any nodes or instances can be managed.
- Cluster credentials (LXD API endpoint and TLS certificates) are stored securely in the database (encrypted at rest).
- The manager's internal concurrency model must ensure that per-cluster reconciliation goroutines are isolated and observable.
- Cross-cluster migration or federation can be addressed in a future ADR if the requirement emerges.

## Related ADRs

- ADR-005 — Hyperscaler Integration Approach
- ADR-006 — Orchestration and Scheduling Strategy
- ADR-007 — Live Migration Mechanism
