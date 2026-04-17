# ADR-003 — Authentication and Authorization Strategy

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

The manager API is internal-only — it is accessed by trusted clients (CLI tools, internal automation, dashboards) within a controlled network. There are no third-party or end-user consumers. The authentication mechanism must be secure, simple to operate, and not require external identity infrastructure.

## Decision

The manager API uses **static API keys** passed as a bearer token in the `Authorization` header.

- Each client is issued a unique API key.
- Keys are stored server-side as hashed values (bcrypt or Argon2).
- Keys are configured via environment variables or a secrets manager (not committed to source control).
- HTTPS/TLS is required for all API communication to protect keys in transit.

## Rationale

- **Simplicity:** API keys require no external identity provider, token exchange flows, or certificate infrastructure, making them operationally lightweight for an internal service.
- **Sufficient for the threat model:** With HTTPS enforced and a controlled network boundary, API keys provide adequate security without OAuth2/OIDC complexity.
- **Easy to rotate:** New keys can be issued and old ones revoked without redeploying clients.
- **Audit-friendly:** Per-client keys enable request attribution in logs.

## Alternatives Considered

- **mTLS:** Stronger mutual authentication, but requires managing client certificates and a PKI — significant operational overhead for an internal tool.
- **OAuth2 / OIDC:** Appropriate for user-facing or multi-tenant systems, but overly complex for a purely internal service with no user identity requirements.
- **Shared secret (single key for all clients):** Simpler, but provides no per-client attribution and makes rotation riskier (all clients affected simultaneously).

## Consequences

- All API requests must include a valid `Authorization: Bearer <api-key>` header; unauthenticated requests return `401`.
- API keys must never be committed to source control; a `.env.example` template documents required variables.
- Key hashing, validation, and rotation logic is implemented in the manager's authentication middleware.
- If multi-tenancy or user-facing access is added in the future, this ADR should be revisited in favour of OAuth2/OIDC.
