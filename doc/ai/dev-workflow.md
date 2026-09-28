---
name: lokol-dev-workflow
description: >-
  Development workflow, Beads (bd) issue tracking protocol, PR rules, ADR standards, and quality gates.
  Use when creating issues, preparing pull requests, or authoring Architecture Decision Records.
---

# Development Workflow & PR Protocol

## Core Rules for Autonomous Agents
1. **Never commit directly to `main`**: Always create a descriptive feature branch (`feat/*`, `fix/*`, `docs/*`) and open a PR via `gh pr create`.
2. **Quality Gates are Non-Negotiable**: Run `make test` before opening any PR; all hermetic unit tests across all subprojects must pass.
3. **Zero-Python Production Runtime Policy**: Production code in `liblokol/`, `cmd/lk/`, `cmd/lokol/`, and `cmd/lokol-mcp/` must be 100% pure Go ([ADR 0021](../adr/0021-zero-python-production-runtime-dependency-policy.md)). Python is allowed only in `tools/` and must be managed via `uv` with PEP 723 metadata ([ADR 0015](../adr/0015-auxiliary-tooling-isolation-via-uv.md)).
4. **License & Attribution**: Maintain the standard Boggy Creek Software LLC MIT license header across all new Go source files.

## Beads (`bd`) Issue Tracking Protocol
This repository uses **bd** for durable task tracking:
- `bd ready`: Find unblocked work.
- `bd show <id>`: Inspect task details.
- `bd update <id> --claim`: Claim work atomically.
- `bd close <id>`: Mark work completed.
- `bd dolt push`: Sync Dolt issues database with git remote.

### Critical Rule: Filing vs. Implementation Authorization
- **Submitting/Filing (`bd create`)**: When asked to create or file a bead, submit it with title, description, priority, and labels. **DO NOT** claim or immediately begin implementing. Wait for explicit authorization.
- **Authorization to Implement**: Only claim and begin implementing when the operator explicitly instructs you to proceed (e.g. "claim and implement", "let's do that now").

## Architecture Decision Record (ADR) Standards
When authoring or amending ADRs in `doc/adr/`:
1. **Strictly Single-Topic ONLY**: Each ADR addresses exactly one architectural decision. Never combine orthogonal concerns.
2. **Audit Existing ADRs First**: Inspect `doc/adr/README.md` and ensure the decision is not already accepted.
3. **What and Why, No How**: Express context, motivation, architectural decision, and trade-offs. Never include perishable implementation details (code snippets, struct signatures, transient flag names, internal package paths).
4. **Machine-Readable YAML Front Matter**: Every ADR must include standardized front matter (`adr`, `title`, `topic`, `theme`, `status`, `version`, `as_built`, `tags`, `executive_summary`).

Further reading:
- [ADR 0015 — Auxiliary Developer Tooling Isolation via Virtual Environments](../adr/0015-auxiliary-tooling-isolation-via-uv.md)
- [ADR 0018 — Monorepo Workspace Architecture with Subproject Isolation](../adr/0018-monorepo-workspace-architecture-with-subproject-isolation.md)
- [ADR 0021 — Zero-Python Production Runtime Dependency Policy](../adr/0021-zero-python-production-runtime-dependency-policy.md)
- [ADR 0022 — Colocated Unit Testing and Integration Hierarchy](../adr/0022-colocated-unit-testing-and-integration-hierarchy.md)
