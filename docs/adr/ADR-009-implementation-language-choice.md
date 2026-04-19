# ADR-009 — Implementation Language Choice

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The manager is a long-running service responsible for cluster orchestration, API serving, background reconciliation loops, and integration with LXD, PostgreSQL, and hyperscaler REST APIs. The language choice affects developer ergonomics, ecosystem fit, runtime performance, operational simplicity, and long-term maintainability.

## Decision

The manager is implemented in **Go**.

## Rationale

| Criterion | Go |
|-----------|-----|
| LXD ecosystem | LXD is itself written in Go; the official LXD client library (`github.com/canonical/lxd`) is Go-native |
| Hyperscaler SDKs | Official Go SDKs for major hyperscalers (e.g. `hcloud-go` for Hetzner Cloud) — first-class support |
| PostgreSQL | Excellent drivers (`pgx`) and migration tools (`golang-migrate`) |
| Concurrency | Goroutines and channels are a natural fit for per-cluster reconciliation loops running in parallel |
| Single binary deployment | Go compiles to a self-contained static binary — no runtime dependency, easy to containerise |
| Performance | Compiled language with low memory footprint; suitable for a long-running service |
| Operational simplicity | No interpreter, no virtual environment, straightforward cross-compilation |
| Team familiarity | Go is widely known; tooling (`gofmt`, `go vet`, `golangci-lint`) enforces consistency automatically |

## Alternatives Considered

- **Rust:** Strong performance and memory safety guarantees, but significantly higher development complexity and a smaller ecosystem for LXD and hyperscaler SDK integrations. Better suited to systems programming than service orchestration.
- **Python:** Rapid prototyping and a large library ecosystem, but dynamic typing and the GIL make concurrent reconciliation loops more complex to manage reliably. Less ergonomic for producing a deployable single binary.
- **Java / Kotlin:** Mature ecosystems, but JVM startup time and memory overhead are unnecessary for this use case. No native LXD client library.
- **TypeScript / Node.js:** No native LXD client; event-loop concurrency is less natural for CPU-adjacent scheduling logic.

## Consequences

- All manager code is written in Go; the minimum supported version is tracked in `go.mod`.
- Code style is enforced by `gofmt` and `golangci-lint` in CI.
- The project structure follows standard Go conventions (`cmd/`, `internal/`, `pkg/`).
- Contributors unfamiliar with Go are expected to follow the language conventions documented in `CONTRIBUTING.md`.
- If a component with substantially different requirements emerges (e.g. a web dashboard), it may use a different language in a separate module — this ADR covers the manager service only.

## Related ADRs

- ADR-002 — API Design Style
- ADR-005 — Hyperscaler Integration Approach
- ADR-006 — Orchestration and Scheduling Strategy
