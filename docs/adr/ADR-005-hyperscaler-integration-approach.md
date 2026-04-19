# ADR-005 — Hyperscaler Integration Approach

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The manager must provision and deprovision cloud servers (future LXD nodes) as part of its auto-scaling behaviour. The initial target hyperscaler is **Hetzner Cloud**, but the design must remain extensible to additional providers (e.g. AWS, DigitalOcean, OVH) without rewriting the orchestration logic.

Server provisioning must be idempotent and recoverable in the event of a partial failure. Terraform is explicitly excluded as a dependency. No persistent state backend is required from the operator.

## Decision

Use the **Pulumi Automation API** (embedded Go library) to provision and deprovision cloud servers, with a **Hetzner Cloud provider** as the first implementation and a **provider abstraction interface** that future hyperscalers implement.

The abstraction interface exposes the following operations:

```
ProvisionServer(ctx, spec) → ServerID
DeprovisionServer(ctx, ServerID) → error
GetServer(ctx, ServerID) → ServerInfo
ListServers(ctx) → []ServerInfo
```

Each provisioning operation runs in a transient, self-managed workspace that is created at the start of the operation and removed automatically when it completes. No persistent state backend needs to be configured or managed by the operator.

Idempotency is achieved through Pulumi's declarative model: the program describes the desired infrastructure state and the provider's API reconciles it (e.g. the Hetzner Cloud provider will not create a duplicate server if one with the same name already exists). This eliminates manual drift-correction steps from the provisioning code path.

## Rationale

- **Pulumi Automation API** embeds directly into a Go binary — no separate CLI, HCL, or external process required. Provisioning logic runs in-process with the manager.
- **Stateless operation**: each call is idempotent by design. The transient workspace approach means no state backend needs to be provisioned, configured, or backed up. This removes an entire operational concern.
- **Idempotency without state management**: Pulumi's declarative model combined with the provider's idempotent API (e.g. Hetzner Cloud's server creation API) ensures that repeated calls for the same desired state produce the same outcome without manual intervention.
- **Provider extensibility** is a first-class Pulumi concept — adding a new hyperscaler means implementing a new Pulumi provider stack, not rewriting provisioning logic.
- **Hetzner Cloud** has an official Pulumi provider (`pulumi-hcloud`) that is actively maintained.
- No Terraform: avoids the HCL DSL, Terraform binary dependency, and the operational complexity of managing a Terraform state backend separately from the manager.

## Alternatives Considered

- **Pure REST API (Hetzner SDK):** Lightweight and no external dependency, but requires hand-rolling idempotency, retry logic, and a full new client for every additional hyperscaler. Does not scale to the multi-hyperscaler requirement.
- **Terraform via `exec.Command`:** Excluded by requirement. Also introduces HCL as a second language and a binary dependency.
- **Crossplane:** Kubernetes-native, powerful, but requires a Kubernetes control plane as a dependency — inappropriate for a standalone service.
- **Pulumi with a persistent state backend:** Considered and rejected. Requires operators to provision and manage a state storage backend (local filesystem or object store), adds an operational dependency, and introduces drift between the manager's view of state and the actual cloud state across process restarts. The stateless transient approach is simpler and equally correct for the current use case.

## Consequences

- Pulumi Go SDK is added as a dependency (`github.com/pulumi/pulumi/sdk/v3`).
- No state backend configuration is required from operators — no environment variables for state storage.
- The `HyperscalerProvider` interface is defined in the manager codebase; the Hetzner implementation is the first concrete provider.
- Adding a new hyperscaler requires implementing the provider interface and a corresponding Pulumi stack definition — no changes to orchestration logic.
- Each provisioning call is self-contained: a transient temporary directory is created, used, and deleted automatically. This is safe for concurrent calls.

## Related ADRs

- ADR-006 — Orchestration and Scheduling Strategy
- ADR-008 — Multi-Cluster Management Model
- ADR-009 — Implementation Language Choice
