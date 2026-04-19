# ADR-005 — Hyperscaler Integration Approach

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |
| Updated | 2026-04-19 |

## Context

The manager must provision and deprovision cloud servers (future LXD nodes) as part of its auto-scaling behaviour. The initial target hyperscaler is **Hetzner Cloud**, but the design must remain extensible to additional providers (e.g. AWS, DigitalOcean, OVH) without rewriting the orchestration logic.

Server provisioning must be idempotent and recoverable in the event of a partial failure or a crash mid-operation. Terraform is explicitly excluded as a dependency. No persistent state backend is required from the operator.

## Decision

Use the **hyperscaler's own REST API** (via the official Go SDK) to provision and deprovision cloud servers, with a **Hetzner Cloud provider** as the first implementation and a **provider abstraction interface** that future hyperscalers implement.

The abstraction interface exposes the following operations:

```
ProvisionServer(ctx, spec) → ServerID
DeprovisionServer(ctx, ServerID) → error
GetServer(ctx, ServerID) → ServerInfo
ListServers(ctx) → []ServerInfo
```

`ProvisionServer` uses an idempotent **check-then-create** pattern:

1. Look up the server by name via `GetByName`. If it exists, return its ID immediately.
2. If it does not exist, create it via `Create` and return the new ID.

No external state files, state backends, or provisioning engines are involved. Every operation reads current truth directly from the hyperscaler API.

## Rationale

- **No external state**: the hyperscaler API is the single source of truth. There are no state files to lose, corrupt, or restore after a failure — re-calling `ProvisionServer` after a crash is always safe.
- **Idempotency by design**: the check-then-create pattern means repeated calls for the same server name produce the same outcome with no manual intervention.
- **Operational simplicity**: operators need only the hyperscaler API token. No additional backend, directory, or environment variable is required for state storage.
- **Small dependency surface**: only the hyperscaler's own Go SDK is needed (`hcloud-go` for Hetzner). No provisioning engine or plugin ecosystem.
- **Provider extensibility**: each hyperscaler implements the `HyperscalerProvider` interface. Adding a new provider requires only a new implementation; no changes to orchestration logic.
- No Terraform: avoids the HCL DSL, Terraform binary dependency, and the operational complexity of managing a Terraform state backend.

## Alternatives Considered

- **Pulumi Automation API:** Previously used in this codebase. Requires an in-process or external state backend; losing state files while cloud resources exist degrades the management plane and requires manual recovery (`pulumi state import`). The REST API approach achieves the same idempotency guarantee without any of that operational burden.
- **Terraform via `exec.Command`:** Excluded by requirement. Also introduces HCL as a second language and a binary dependency.
- **Crossplane:** Kubernetes-native, powerful, but requires a Kubernetes control plane as a dependency — inappropriate for a standalone service.

## Consequences

- No provisioning engine dependency is required — only the hyperscaler's own Go SDK.
- No state backend configuration is required from operators.
- The `HyperscalerProvider` interface is defined in the manager codebase; the Hetzner implementation is the first concrete provider.
- Adding a new hyperscaler requires implementing the provider interface using the new hyperscaler's SDK — no changes to orchestration logic.
- All four operations (`ProvisionServer`, `DeprovisionServer`, `GetServer`, `ListServers`) are self-contained REST calls. They are safe for concurrent use.

## Related ADRs

- ADR-006 — Orchestration and Scheduling Strategy
- ADR-008 — Multi-Cluster Management Model
- ADR-009 — Implementation Language Choice
