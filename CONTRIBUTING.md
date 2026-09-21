# Contributing to lokol

Thank you for your interest in `lokol`! This document provides governance guidelines and complete instructions for configuring your local development environment, managing multiple Go toolchain versions under XDG paths, diagnosing environment health via `--doctor`, executing the test suite, and tracking tasks using Beads.

---

## 1. Governance & External Contributions

At this time, **`lokol` is maintained solely by Boggy Creek Software LLC and is not accepting external pull requests or feature requests**. Public contributions and PRs are disabled to maintain strict security boundaries and development velocity.

If you are inspecting, developing, or maintaining a fork of `lokol`, feel free to use and modify the code under the terms of the [MIT License](LICENSE). The instructions below detail the environment setup and tooling used to build and test the project.

---

## 2. Automated Development Environment Setup (`setup.sh`)

To streamline onboarding across different operating systems and developer environments, `lokol` provides a dedicated setup script: [`setup.sh`](setup.sh).

The script automatically detects your operating system, CPU architecture, and available package manager, then installs and configures all required toolchains and dependencies.

### Quick Start

From the root of the repository, execute:

```bash
./setup.sh
```

For unattended or automated environments (e.g. CI containers, Docker builds, cloud workstations), pass the `-y` flag to run without prompting:

```bash
./setup.sh -y
```

---

## 3. Environment Diagnostics (`setup.sh --doctor`)

To verify whether your system has all required tools, proper directory permissions, and valid XDG paths, run the built-in diagnostic doctor:

```bash
./setup.sh --doctor
```

### What `--doctor` Inspects:
1. **Operating System & Architecture**: Platform (`darwin` / `linux`), kernel release, and CPU architecture (`amd64` / `arm64`).
2. **XDG Base Directory Compliance**:
   - `XDG_BIN_HOME` (`~/.local/bin`), `XDG_DATA_HOME` (`~/.local/share`), `XDG_CONFIG_HOME` (`~/.config`), and `XDG_CACHE_HOME` (`~/.cache`).
   - PATH inclusion of `~/.local/bin`.
   - Security permissions of the `.beads` directory (verifies `0700` mode).
3. **Go SDKs & Multi-Version Toolchains**:
   - Active Go binary path and resolved version (`go version`).
   - Compatibility check against the version required by `go.mod` (e.g., Go 1.25.8).
   - Inventory of all installed XDG Go SDKs and highlights the active one.
4. **Core Build Tools**: Presence and versions of `git`, `make`, C compiler (`gcc` or `clang`), `cmake`, and `pkg-config`.
5. **Beads Issue Tracker**: Validates that `bd` is executable and the local database is initialized.
6. **Inference Engine (`llama-server`)**: Checks for `llama-server` / `llama` binary and probes connectivity to `http://127.0.0.1:8080`.
7. **Actionable Remediation**: If any checks produce `[WARN]` or `[FAIL]`, the doctor outputs exact shell commands to resolve the issue.

---

## 4. Supported Platforms & Package Managers

`setup.sh` has first-class support for three primary package management ecosystems:

| Platform / Distro Family | Package Manager | Detected By | Default Packages Installed |
| :--- | :--- | :--- | :--- |
| **macOS (Darwin)** | Homebrew (`brew`) | `uname -s == Darwin` | `git`, `curl`, `make`, `pkg-config`, `cmake`, Go SDK |
| **Linux (APT-based)** | APT (`apt-get`) | `apt-get` on PATH (Ubuntu, Debian, Pop!_OS, Linux Mint) | `curl`, `git`, `make`, `build-essential`, `pkg-config`, `tar`, `gzip`, `ca-certificates`, `cmake`, Go SDK |
| **Linux (RPM-based)** | DNF / YUM (`dnf` / `yum`) | `dnf` or `yum` on PATH (Fedora, RHEL, CentOS, Rocky Linux, AlmaLinux) | `curl`, `git`, `make`, `gcc`, `gcc-c++`, `pkgconfig`, `tar`, `gzip`, `ca-certificates`, `cmake`, Go SDK |

---

## 5. Multi-Version Go SDK Management (XDG Paths)

Standard Go multi-version tools pollute `$HOME/sdk`, and system package managers frequently ship outdated Go compilers. `setup.sh` installs and manages Go SDKs entirely in user space adhering strictly to the [XDG Base Directory Specification](doc/adr/0006-xdg-base-directory-specification.md):

### Directory Layout

```
~/.local/
├── bin/
│   ├── go              -> ~/.local/share/go/sdk/current/bin/go       # Active Go binary
│   ├── gofmt           -> ~/.local/share/go/sdk/current/bin/gofmt    # Active gofmt
│   ├── go1.25.8        -> ~/.local/share/go/sdk/go1.25.8/bin/go      # Direct version alias
│   ├── go1.24.1        -> ~/.local/share/go/sdk/go1.24.1/bin/go      # Direct version alias
│   └── bd                                                            # Beads issue tracker CLI
│
└── share/
    └── go/
        └── sdk/
            ├── current -> go1.25.8                                   # Active SDK pointer
            ├── go1.25.8/                                             # Full Go 1.25.8 SDK
            └── go1.24.1/                                             # Full Go 1.24.1 SDK
```

### Managing Go Versions

- **Install a specific Go version** (e.g. 1.25.8):
  ```bash
  ./setup.sh --go-version 1.25.8
  ```
- **List installed Go versions**:
  ```bash
  ./setup.sh --list-go
  ```
- **Switch active Go version**:
  ```bash
  ./setup.sh --switch-go 1.24.1
  ```
- **Invoke a specific version directly**:
  ```bash
  go1.25.8 version
  go1.24.1 test ./...
  ```

---

## 6. Options and Flags for `setup.sh`

```bash
Usage: setup.sh [options]

Options:
  -y, --yes, --non-interactive   Run non-interactively (assumes 'yes' to package installs)
  --doctor                       Run comprehensive development environment diagnostic checks
  --go-version <version>         Install and activate specific Go SDK version (e.g. 1.25.8)
  --list-go                      List all installed XDG Go SDK versions
  --switch-go <version>          Switch active Go SDK to an already installed version
  --no-sudo                      Do not invoke sudo (for rootless or unprivileged environments)
  --skip-go                      Skip Go toolchain installation/check
  --skip-beads                   Skip Beads (bd) issue tracking CLI installation
  --skip-cmake                   Skip CMake build tool installation
  --dry-run                      Print actions and commands without executing them
  -h, --help                     Display usage help
```

---

## 7. Building & Testing `lokol`

Once dependencies are installed and `~/.local/bin` is in your `PATH`, verify the build:

### 1. Run Unit Tests
```bash
make test
```
Runs all unit and integration tests under `./test/...` with race detection.

### 2. Build the lokol Binary
```bash
make build
```
Compiles a static binary into `bin/lokol` with build date and Git commit metadata injected via `ldflags`.

### 3. Run Hardware Probe
```bash
make probe
```
Executes hardware detection to verify CPU vector extensions and GPU VRAM tiers.

### 4. Clean Build Artifacts
```bash
make clean
```
Removes `bin/` and `dist/` directories.

---

## 8. Issue & Task Tracking Workflow (`bd`)

`lokol` uses **Beads (`bd`)** for issue and task tracking. Do not create markdown TODO files or external task lists.

### Basic Beads Commands

```bash
# Refresh session rules and context
bd prime

# List unblocked issues ready to be worked on
bd ready

# View issue details and dependencies
bd show <issue-id>

# Claim an issue before starting implementation
bd update <issue-id> --claim

# Create a new issue or follow-up task
bd create --title="Feature description" --description="Why this exists and what needs to be done" --type=task --priority=2

# Close an issue upon passing quality gates
bd close <issue-id> --reason="Completed and verified via tests"
```

---

## 9. Standards & Conventions

1. **Copyright & License Header**: Every shell and Go file must start with the Boggy Creek Software LLC MIT license header:
   ```go
   // Copyright (c) 2026 Boggy Creek Software LLC
   //
   // Use of this source code is governed by an MIT-style
   // license that can be found in the LICENSE file.
   ```
2. **XDG Compliance**: Respect the XDG Base Directory specification ([ADR-0006](doc/adr/0006-xdg-base-directory-specification.md)). Do not create dotfiles in `$HOME`.
3. **Non-Interactive Shell Scripts**: Always use `-f`, `-rf`, and non-interactive flags (`apt-get -y`, `HOMEBREW_NO_AUTO_UPDATE=1`, `rm -rf`, `cp -f`) to prevent scripts and agents from hanging on confirmation prompts.
4. **Formatting**: Run `gofmt -w` on all Go source files prior to testing.
