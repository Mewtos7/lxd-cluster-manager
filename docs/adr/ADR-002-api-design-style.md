# ADR-002 — API Design Style

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The LX Container Weaver manager exposes an API for CRUD operations on clusters, nodes, containers, and VMs. The API is consumed exclusively by internal clients (CLI tools, dashboards, and internal services) — no third-party or public consumers are expected. The choice of API style affects developer ergonomics, tooling support, performance, and long-term maintainability.

## Decision

The manager API is designed as a **RESTful HTTP API** using JSON as the wire format, versioned under a `/v1/` path prefix.

## Rationale

- **REST is universally understood** and requires no code generation or special tooling for initial development and testing.
- **Internal-only consumers** reduce the pressure to use gRPC for cross-language contract enforcement — all clients will be part of the same repository or ecosystem.
- **Hetzner Cloud API and LXD API are both REST-based**, keeping the integration surface consistent and reducing the number of protocol concepts in the codebase.
- REST over HTTPS with JSON is straightforward to inspect, debug, and document with tools like OpenAPI/Swagger.
- Performance requirements (provisioning, scaling, migration operations) are dominated by infrastructure latency, not API serialization overhead, making gRPC's binary efficiency advantage negligible here.

## Alternatives Considered

- **gRPC:** Strong contract enforcement and better streaming support, but adds code generation complexity and is less ergonomic for ad hoc testing and scripting. Preferred if clients become cross-language or performance-critical.
- **GraphQL:** Flexible querying, but adds significant complexity for an API with well-defined, bounded resource types. Overkill for this use case.

## Consequences

- API surface is documented with an OpenAPI specification committed to the repository.
- All resources follow standard REST conventions: `GET`, `POST`, `PUT`/`PATCH`, `DELETE` with consistent status code usage.
- API versioning (e.g. `/v1/`) is included from the start to allow non-breaking evolution.
- If streaming requirements emerge (e.g. real-time node metrics), Server-Sent Events (SSE) or WebSockets may be added alongside REST without replacing it.
