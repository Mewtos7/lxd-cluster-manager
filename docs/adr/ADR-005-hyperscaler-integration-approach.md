# ADR-005 — Hyperscaler Integration Approach

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The manager must provision and deprovision cloud servers (future LXD nodes) as part of its auto-scaling behaviour. The initial target hyperscaler is **Hetzner Cloud**, but the design must remain extensible to additional providers (e.g. AWS, DigitalOcean, OVH) without rewriting the orchestration logic.

Server provisioning must be idempotent, state-tracked, and recoverable in the event of a partial failure. Terraform is explicitly excluded as a dependency.

## Decision

Use the **Pulumi Automation API** (embedded Go library) to provision and deprovision cloud servers, with a **Hetzner Cloud provider** as the first implementation and a **provider abstraction interface** that future hyperscalers implement.

The abstraction interface exposes the following operations:

```
ProvisionServer(ctx, spec) → ServerID
DeprovisionServer(ctx, ServerID) → error
GetServer(ctx, ServerID) → ServerInfo
ListServers(ctx) → []ServerInfo
```

The Pulumi stack per-cluster manages the lifecycle of servers declared for that cluster.

## Rationale

- **Pulumi Automation API** embeds directly into a Go binary — no separate CLI, HCL, or external process required. Provisioning logic runs in-process with the manager.
- **State management and idempotency** are handled by Pulumi natively, including drift detection and rollback on failure. This is critical in an orchestrator that may retry or crash mid-provision.
- **Provider extensibility** is a first-class Pulumi concept — adding a new hyperscaler means implementing a new Pulumi provider stack, not rewriting provisioning logic.
- **Hetzner Cloud** has an official Pulumi provider (`pulumi-hcloud`) that is actively maintained.
- No Terraform: avoids the HCL DSL, Terraform binary dependency, and the operational complexity of managing a Terraform state backend separately from the manager.

## Alternatives Considered

- **Pure REST API (Hetzner SDK):** Lightweight and no external dependency, but requires hand-rolling state tracking, idempotency, retry logic, and a full new client for every additional hyperscaler. Does not scale to the multi-hyperscaler requirement.
- **Terraform via `exec.Command`:** Excluded by requirement. Also introduces HCL as a second language and a binary dependency.
- **Crossplane:** Kubernetes-native, powerful, but requires a Kubernetes control plane as a dependency — inappropriate for a standalone service.

## Consequences

- Pulumi Go SDK is added as a dependency (`github.com/pulumi/pulumi/sdk/v3`).
- Each cluster has an associated Pulumi stack; stack state is stored in a configured Pulumi backend (local filesystem for development, object storage for production).
- The `HyperscalerProvider` interface is defined in the manager codebase; the Hetzner implementation is the first concrete provider.
- Adding a new hyperscaler requires implementing the provider interface and a corresponding Pulumi stack definition — no changes to orchestration logic.
- Pulumi state backends (e.g. S3-compatible object store) must be configured and documented for production deployments.

## Related ADRs

- ADR-006 — Orchestration and Scheduling Strategy
- ADR-008 — Multi-Cluster Management Model
- ADR-009 — Implementation Language Choice
