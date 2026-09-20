# ADR 0010: Persistent Memory Storage Architecture in XDG State and Data Directories

## Status
Accepted

## Date
2026-09-20

## Context
Across all operational modes (`general`, `moe`, `coding`), conversational agents suffer from session amnesia: when the process exits, user preferences, established conventions, and intermediate discoveries are permanently lost.

However, saving persistent agent memory raises critical questions regarding filesystem placement:
- Scattering files in the working repository clutters Git trees with developer-specific or uncommitted memories.
- Dumping ad-hoc dotfiles in `$HOME` (e.g. `~/.lokol_memory`) violates modern system hygiene.
- Mixing runtime session state with permanent knowledge complicates backups and cache pruning.

We need a standardized, unprivileged persistence model for cross-session memories.

## Decision
All persistent memories across all `lokol` operational modes will be partitioned and stored strictly within the user's **XDG Base Directory** hierarchy (as established in [ADR-0006](0006-xdg-base-directory-specification.md)):

1. **Persistent Knowledge & Vector Indexes (`XDG_DATA_HOME`)**:
   - Location: `${XDG_DATA_HOME:-~/.local/share}/lokol/memory/`
   - Contains durable memories: user preferences, extracted facts, repository architecture summaries, and vector index collections.
   - Preserved across sessions and updates; never deleted during routine cache clearing.
2. **Session History & Telemetry (`XDG_STATE_HOME`)**:
   - Location: `${XDG_STATE_HOME:-~/.local/state}/lokol/sessions/`
   - Contains turn logs, prompt journals, and execution metadata that can be audited or truncated without losing durable knowledge.
3. **Volatile Caches (`XDG_CACHE_HOME`)**:
   - Location: `${XDG_CACHE_HOME:-~/.cache}/lokol/embeddings/`
   - Contains transient embedding caches and temporary token calculations that can be wiped safely at any time.

## Consequences

### Positive
- **Clean Workspace**: Zero memory files or temporary state written to user git repositories.
- **Predictable Lifecycle**: Users and system administrators can back up durable memories (`~/.local/share/lokol/memory`), inspect session journals (`~/.local/state/lokol`), or purge transient caches (`~/.cache/lokol`) independently.
- **Rootless & Secure**: Operates entirely within standard user filesystem permissions.

### Negative / Trade-offs
- Sharing memories across a team requires an explicit export/import or sync command rather than committing memory files directly to Git.
