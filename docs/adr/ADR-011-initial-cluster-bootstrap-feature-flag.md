# ADR-011 — Initial Cluster Bootstrap Feature Flag and Guard

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-20 |

## Context

The manager must be able to bootstrap the very first LXD cluster automatically at startup. However, unconditional bootstrap execution is dangerous: if the logic runs on every startup it will attempt to re-initialise an already healthy cluster, and if it runs on a manager that has already provisioned clusters it will create duplicates or fail noisily.

Two conditions must always be checked before bootstrap can proceed:

1. The operator has explicitly opted in (a feature flag), because bootstrap is a high-impact, one-time operation.
2. No cluster has been registered yet, because bootstrap must be idempotent with respect to existing state.

The manager must also emit structured log messages at startup so that operators can diagnose exactly why bootstrap did or did not run.

## Decision

Introduce a boolean feature flag `INITIAL_BOOTSTRAP_ENABLED` (default `false`) and a **`Guard`** component in `internal/bootstrap/guard.go` that evaluates the two guard conditions in order at startup:

1. **Flag check** — if `INITIAL_BOOTSTRAP_ENABLED` is `false`, log `"initial bootstrap disabled; skipping"` and return immediately (no repository access).
2. **Cluster existence check** — call `ClusterRepository.ListClusters`. If any cluster already exists, log `"skipping initial bootstrap: cluster already exists"` and return.
3. **Bootstrap invocation** — if both checks pass, log `"no existing clusters found; invoking bootstrap coordinator"` and call `BootstrapCoordinator.RunBootstrap`.

The guard accepts a `BootstrapCoordinator` interface, keeping it decoupled from any concrete provisioning logic. A `BootstrapCoordinatorFunc` adapter allows plain functions to satisfy the interface without a dedicated struct.

## Rationale

- **Explicit opt-in by default**: bootstrap is a one-time, potentially destructive operation. Defaulting to `false` ensures the manager is safe to restart or redeploy without risk of accidental cluster reinitialisation.
- **Repository check as a hard guard**: even when the flag is `true`, the manager re-reads the cluster repository at startup. This makes the guard resilient to operator error (forgetting to reset the flag after the first bootstrap) and safe across rolling restarts.
- **Structured logging at every branch**: every exit path emits a distinct log message, giving operators a clear audit trail of startup decisions without inspecting code.
- **Interface-based coordinator**: decoupling the guard from the coordinator keeps the guard unit-testable with a simple stub and avoids tight coupling to the full bootstrap workflow.
- **Determinism when disabled**: when `INITIAL_BOOTSTRAP_ENABLED=false` the guard performs zero I/O, making startup fully side-effect free and deterministic.

## Alternatives Considered

- **Always attempt bootstrap and rely on idempotency**: the bootstrapper is already idempotent, but this would cause unnecessary repository reads and LXD API calls on every startup, and would make the intent unclear to operators.
- **Bootstrap on first API call rather than at startup**: defers the operation but complicates observability and makes it harder to detect failure at startup, where structured logs are most visible.
- **A config-file flag instead of an environment variable**: inconsistent with the rest of the configuration model (twelve-factor, env-only). See ADR-009.

## Consequences

- `INITIAL_BOOTSTRAP_ENABLED` is documented in `.env.example` and in the README environment variable table.
- The `Guard` type is the single authoritative entry point for initial bootstrap. All other code paths must not call bootstrap directly at startup.
- Unit tests cover all three guard branches (flag disabled, flag enabled + cluster exists, flag enabled + no cluster).
- Adding `INITIAL_BOOTSTRAP_ENABLED=true` to a running production environment is safe: the guard will check the repository and skip bootstrap if any cluster already exists.

## Related ADRs

- ADR-005 — Hyperscaler Integration Approach
- ADR-006 — Orchestration and Scheduling Strategy
- ADR-008 — Multi-Cluster Management Model
