# ADR 0006: XDG Base Directory Standard Adoption

## Status
Accepted

## Date
2026-09-20

## Context
Command-line applications and local AI tooling frequently clutter user home directories with ad-hoc hidden directories (e.g. `~/.lokol`, `~/.models`, `~/.config_lokol`), violating standard filesystem layout conventions. Additionally, installation scripts often assume root or `sudo` privileges to write to `/usr/local/bin` or `/usr/bin`, which fails in restricted developer environments, CI containers, rootless container setups, or shared multi-user workstations.

Furthermore, running local models requires managing three distinct classes of persistent files:
1. **Executable Binaries**: `lokol`, `lokol-mcp`, and inference engines like `llama` / `llama-server`.
2. **Configuration Files**: User preferences, default engine URLs, model tier overrides, and keybindings.
3. **Bulky Data / Model Weights**: Multi-gigabyte GGUF model files (e.g., 2GB to 5GB per model checkpoint).
4. **Runtime State / Cache**: Temporary sockets, session history, logs, and token cache files.

We need a standardized, unprivileged, predictable directory layout that aligns with modern Linux and macOS conventions.

## Decision
We adopt the **XDG Base Directory Specification** as the official standard for binary distribution, installation scripts, configuration, and data storage in `lokol`.

### Directory Layout Mapping

| Component | Environment Variable | Default Location | Purpose |
| :--- | :--- | :--- | :--- |
| **Executables** | `XDG_BIN_HOME` | `~/.local/bin` | Installed `lokol` binary, `lokol-mcp`, and user-compiled binaries. |
| **Configuration** | `XDG_CONFIG_HOME` | `~/.config/lokol` | `config.toml` or `config.json`, profile definitions, overrides. |
| **Data & Weights** | `XDG_DATA_HOME` | `~/.local/share/lokol` | Downloaded GGUF models (`models/`), persistent vector databases, indices. |
| **State & Logs** | `XDG_STATE_HOME` | `~/.local/state/lokol` | Multi-turn session logs, command history, runtime telemetry. |
| **Cache** | `XDG_CACHE_HOME` | `~/.cache/lokol` | AST symbol caches, temporary refinery indexes, token counters. |

### Implementation Details
1. **Non-Root Installation via `install.sh`**:
   - The installer installs directly into `${XDG_BIN_HOME:-$HOME/.local/bin}` without requiring `sudo`.
   - The installer automatically creates `${XDG_DATA_HOME:-$HOME/.local/share/lokol}` and `${XDG_CONFIG_HOME:-$HOME/.config/lokol}` with appropriate user permissions (`0755`).
   - The installer checks if `${TARGET_BIN_DIR}` is in `$PATH` and prints export guidance if absent (`export PATH="${HOME}/.local/bin:${PATH}"`).
2. **Cross-Platform Compatibility (Linux & macOS)**:
   - On Linux, standard XDG environment variables are respected.
   - On macOS (Darwin), `~/.local/bin` and `~/.local/share/lokol` are also used by default to maintain consistent shell and container developer ergonomics, while allowing explicit override via standard XDG environment variables.
3. **Fallback & Legacy Model Discovery**:
   - To avoid redundant downloads for existing local setups, `lokol` and `lokol setup` check `${XDG_DATA_HOME}/lokol/models/` first, but gracefully fallback to checking legacy `~/models/` if present.

## Consequences

### Positive
- **Rootless & Secure**: Zero `sudo` or root permissions required for installation or updates.
- **Clean Home Directory**: Prevents "dotfile pollution" in `$HOME`.
- **Easy Backup & Pruning**: System administrators and users can wipe caches (`rm -rf ~/.cache/lokol`) or backup configurations (`~/.config/lokol`) independently of multi-gigabyte model weights (`~/.local/share/lokol`).
- **Container & Sandbox Friendly**: Easily mountable into `agent-sandbox` containers via standard user volume mappings.

### Negative / Trade-offs
- Users who do not have `~/.local/bin` in their default `$PATH` must append one line to `~/.bashrc` or `~/.zshrc`. The installer detects this condition and provides the exact command.
