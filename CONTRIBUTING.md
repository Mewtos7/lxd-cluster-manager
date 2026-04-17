# Contributing

These guidelines apply to all contributors (human and artificial) working on this project.

## Git and GitHub conventions

- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.
- Keep commits small and focused on a single logical change.
- Reference issues in commit messages (for example: `(#4)`) so GitHub links and closes work items correctly.
- Open pull requests with linked issues/work items and a clear scope.

## Code conventions

- Use clear, descriptive names; avoid abbreviations for variables and functions.
- Keep formatting and line breaks consistent with surrounding code.
- Follow language-specific style guidelines and existing project patterns.

## Workflow guidelines

- When changing code, also review and update related:
  - Documentation
  - Guidelines and contributor instructions
  - Dependencies
  - CI/CD pipelines and GitHub Actions

## Legal conventions

- Ensure project and dependency licenses allow:
  - Free software distribution
  - Free commercial use, including selling products built with this software
- Do not introduce dependencies with incompatible license terms.

## Security guidelines

- Design and implement changes to avoid OWASP Top 10 risks.
- Ensure proper authentication and authorization for protected operations.
- Validate inputs, protect secrets, and follow least-privilege principles.

## Architecture Decision Records (ADRs)

Significant architectural changes must be linked to an ADR. A new or updated ADR is required when a pull request:

- Introduces or changes the technology stack, programming languages, frameworks, or libraries.
- Alters API design, authentication strategy, database schema, or data storage approach.
- Changes deployment, orchestration, migration, or multi-cluster management mechanisms.
- Adds, removes, or substantially restructures a major component or service.

**When opening such a PR:**

1. Check whether an existing ADR already covers the decision. If so, reference it (e.g., `ADR-003`) in the PR description.
2. If no existing ADR applies, create a new one under `docs/adr/` using the template defined in `ADR-001` before or alongside the implementation.

See `docs/adr/ADR-001-adr-format-and-storage-convention.md` for the ADR template and naming convention.

## Issue templates

When creating backlog work, use the **User Story** template from `.github/ISSUE_TEMPLATE/user-story.md`.
This keeps user stories consistent with the project workflow and automatically applies the `user-story` label.
