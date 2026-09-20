# ADR 0007: Generalized Personas and Model Selection Matrix (General, MoE, Coding)

## Status
Accepted

## Date
2026-09-20

## Context
Initial iterations of `lokol` targeted autonomous agentic coding exclusively, defaulting hardware probing and engine configurations to code-specialized models (such as `Qwen 2.5 Coder 7B/3B`). However, users require `lokol` across diverse operational contexts:
1. **General-Purpose Mode (Default)**: Daily conversational assistance, summarization, research, brainstorming, and writing assistance, where coding-dense system prompts and tool schemas waste context tokens and degrade prose generation.
2. **Mixture of Experts (MoE) Mode**: Sparse activation models (e.g. DeepSeek-V2-Lite, Qwen1.5-MoE) where only active expert parameters are routed during token generation, allowing constrained GPUs to achieve superior multi-domain reasoning by loading active expert layers into VRAM while keeping non-activated parameters in host RAM.
3. **Agentic Coding Mode**: Structured software engineering with file system tool schemas, test assertion verifiers, and workspace governance.

In Agentic Coding Mode, `lokol` must also dynamically inherit project-level agent rules and role prompts defined in repository standard files (specifically `AGENTS.md` and fallback `CLAUDE.md`).

## Decision

### 1. Operational Mode Taxonomy
`lokol` introduces an explicit `--mode` flag (and `config.toml` default):

| Mode | Identifier | Default Model Sizing Tier (RTX 3060 12GB) | Primary Focus |
| :--- | :--- | :--- | :--- |
| **General-Purpose** | `general` (Default) | Llama 3.1 8B Instruct / Mistral 7B Instruct (Q4_K_M) | Natural language dialogue, reasoning, summarization, general queries |
| **Mixture of Experts** | `moe` | DeepSeek-V2-Lite / Qwen1.5-MoE-A2.7B | Sparse dynamic expert activation, high-capacity reasoning within constrained VRAM |
| **Agentic Coding** | `coding` | Qwen 2.5 Coder 7B Instruct (Q4_K_M) | Deterministic code editing, testing, tool loops, AST analysis |

The installer (`install.sh`) and `lokol setup` will default to **General-Purpose Mode**, allowing non-developer users to immediately chat without configuring software engineering tools.

### 2. AGENTS.md Contract Ingestion in Coding Mode
When running in `coding` mode (e.g., `lokol chat --mode=coding` or `lokol exec --mode=coding <prompt>`):
1. **Discovery Hierarchy**:
   - `lokol` checks the current working directory for:
     1. `./AGENTS.md` (Standard project instruction contract)
     2. `./CLAUDE.md` (Legacy fallback)
     3. `.github/AGENTS.md`
2. **Context Injection**:
   - If found, the contents of `AGENTS.md` are ingested into the system prompt prefix under a structured `<project_guidelines>` block.
   - Project-specific constraints (e.g., local endpoints, non-interactive shell flags, test verification commands, issue tracking guidelines like Beads) override default agent heuristics.
3. **General Mode Isolation**:
   - In `general` mode, workspace `AGENTS.md` files are ignored unless explicitly referenced by the user, ensuring clean context for everyday conversational tasks.

## Consequences

### Positive
- **Broad Accessibility**: Users without software engineering workflows can use `lokol` out-of-the-box as a private, high-speed local desktop assistant.
- **Hardware-Tailored MoE**: Enables running high-parameter MoE models on consumer GPUs (12GB/8GB) by leveraging llama.cpp's partial offload capabilities.
- **Durable Project Memory & Alignment**: Automatically aligns coding sessions with the repository's ground rules (`AGENTS.md`) without requiring repeated user prompting.

### Negative / Trade-offs
- Multiple downloaded models if a user switches between `general` and `coding` modes (~4.5 GB each). Storage is mitigated by storing weights in `$XDG_DATA_HOME/lokol/models`.
