# ADR 0004: Terminal Interface Strategy (Headless CLI vs Interactive TUI)

## Status
Accepted

## Context
A key goal of `quik` is high performance, responsiveness, and minimal runtime overhead. Users interacting with coding agents have two distinct usage modes:
1. **Interactive Agent Mode**: The developer chats with the agent, inspects diffs, approves/rejects bash execution commands, and watches token streaming live.
2. **Headless / Automation Mode**: The agent is invoked via scripts, CI/CD, Git hooks, or shell pipelines (`quik exec --contract=contract.md`).

A clunky or blocking UI adds friction, while pure stdout streaming can become chaotic when tools, thoughts, and diffs mix together.

## Decision
`quik` will implement a **Dual-Mode UI Architecture**:

1. **Interactive Mode (TUI via Bubble Tea)**:
   - Built on Go's leading terminal UI framework: `charmbracelet/bubbletea`, `lipgloss` (styling), and `bubbles` (components like viewports, spinners, and progress bars).
   - **Split / Tiered Layout**:
     - **Status Header**: Displays active model, GPU VRAM usage bar, context window consumption (e.g. `12,450 / 32,768 tokens`), and generation tokens/sec.
     - **Main Viewport**: Scrollable chat history with syntax-highlighted code blocks, tool executions, and diffs.
     - **Action Banner / Confirmation Prompt**: Clear visual prompt when the agent requests permission to run shell commands (`[Y] Approve  [N] Deny  [E] Edit`).
     - **Input Bar**: Multi-line input with command history.
2. **Headless / Non-TTY Mode (Plain Stream)**:
   - When stdout is not a TTY (or when `--headless` / `--raw` is passed), `quik` gracefully disables the Bubble Tea TUI and outputs pure JSON-lines or standard UNIX streaming text for pipeline integration.

## Consequences
### Positive
- Exceptional user experience: real-time VRAM telemetry, visual confirmation before destructive actions, and clean collapsible tool outputs.
- Pure Go: Bubble Tea compiles directly into the single static binary with no external C dependencies like ncurses.
- Scriptable: Works in headless CI/CD and scripts without modification.
