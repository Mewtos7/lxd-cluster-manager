# ADR-001 — ADR Format and Storage Convention

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

Architecture Decision Records (ADRs) allow the team to document significant technical decisions as they are made, capturing the context, reasoning, and consequences in a durable and reviewable format. Without a defined convention, ADRs may be stored inconsistently, use different templates, or become hard to discover.

## Decision

All ADRs for this project:

- Are stored under `docs/adr/` in the root of the repository.
- Are numbered sequentially starting at `ADR-001`.
- Use the filename pattern `ADR-NNN-short-title-in-kebab-case.md`.
- Follow the section structure defined in this document.

### Template structure

```
# ADR-NNN — Title

| Field  | Value |
|--------|-------|
| Status | Proposed | Accepted | Deprecated | Superseded by ADR-NNN |
| Date   | YYYY-MM-DD |

## Context
## Decision
## Rationale
## Alternatives Considered
## Consequences
```

Valid status values:

| Status | Meaning |
|--------|---------|
| Proposed | Under discussion, not yet adopted |
| Accepted | Agreed upon and in effect |
| Deprecated | No longer applicable |
| Superseded by ADR-NNN | Replaced by a later decision |

## Rationale

- Keeping ADRs in the repository alongside code ensures decisions are version-controlled, diff-able, and discoverable.
- A fixed template reduces cognitive overhead when writing or reviewing ADRs.
- Sequential numbering makes cross-referencing and ordered reading straightforward.

## Alternatives Considered

- **Wiki or external tool (Confluence, Notion):** Decisions would not be co-located with code, creating drift and discoverability issues.
- **Free-form prose without a template:** Harder to review and compare across decisions.

## Consequences

- Every significant architectural decision going forward must produce an ADR before or shortly after implementation.
- When a decision is revised, a new ADR is created and the old one is updated to `Superseded by ADR-NNN`.
- ADR-001 itself is self-referential and acts as the authoritative template.
