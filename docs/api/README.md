# LX Container Weaver — API Specification

The OpenAPI 3.1 specification for the LX Container Weaver manager API is located at:

```
docs/api/openapi.yaml
```

It defines the full REST API contract for clusters, nodes, instances, control actions, scaling actions, migrations, and audit events.

---

## Local Validation

The spec can be validated locally without a running server. Choose **one** of the methods below — no permanent installation is required for the `npx`-based options.

### Option 1 — Python (`openapi-spec-validator`)

Requires Python 3.8+.

```bash
pip install openapi-spec-validator
openapi-spec-validator docs/api/openapi.yaml
```

Expected output on a valid spec:

```
OK
```

### Option 2 — Node.js (`@redocly/cli`)

Requires Node.js 18+. No global install needed.

```bash
npx @redocly/cli lint docs/api/openapi.yaml
```

Expected output on a valid spec:

```
docs/api/openapi.yaml: no errors or warnings found.
```

### Option 3 — Node.js (`swagger-cli`)

Requires Node.js 14+. No global install needed.

```bash
npx swagger-cli validate docs/api/openapi.yaml
```

Expected output on a valid spec:

```
docs/api/openapi.yaml is valid
```

---

## Browsing the Spec

To render the spec as interactive documentation locally:

```bash
npx @redocly/cli preview-docs docs/api/openapi.yaml
```

This starts a local server (default: `http://localhost:8080`) with a rendered view of all endpoints, schemas, and examples.

---

## API Design Conventions

| Convention | Detail |
|---|---|
| Base path | `/v1/` |
| Wire format | JSON (`application/json`) |
| Authentication | `Authorization: Bearer <api-key>` (see [ADR-003](../adr/ADR-003-authentication-and-authorization-strategy.md)) |
| Pagination | `?limit=<n>&offset=<n>` on all list endpoints; responses include `{ items, total }` |
| Error shape | `{ code, message, details }` |
| Cluster-scoped resources | Nested under `/v1/clusters/{cluster_id}/` (see [ADR-008](../adr/ADR-008-multi-cluster-management-model.md)) |

For the full rationale behind these choices, see [ADR-002 — API Design Style](../adr/ADR-002-api-design-style.md).
