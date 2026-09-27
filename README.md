# lokol

`lokol` is a local-first autonomous AI agent engineered to run directly against consumer GPUs without bulky runtime middleware or bloated JSON schema tool harnesses.

## Core Philosophy
1. **Zero Intermediate Abstractions**: Bypass multi-layered Node.js/Python agent frameworks in favor of a lean Go binary and managed `llama-server` process.
2. **Hardware-Aware Tiering**: Automatically probe local hardware (CPU vector extensions, VRAM, system memory) and configure the exact right model, single-slot dedicated `llama-server` (`-np 1`), KV cache quantization, and context window.
3. **Constrained Hardware Support**: First-class support for constrained laptops such as the Lenovo ThinkPad X1 Extreme Gen 1 (GTX 1650 Max-Q 4GB VRAM) via 3B Q4 models with pure-VRAM Q4_0 KV caching.
4. **Deterministic Agent Protocol**: Avoid JSON schema parsing breakdowns on small models by using tagged delimiters (`<action>`) and native Go mechanical tooling (`replace_file`, `write_file`, `exec_bash`).

## Project Structure
```
lokol/
├── bin/                    # Compiled lokol binaries
├── cmd/
│   ├── lokol/              # CLI entrypoint (setup, probe, chat, exec, version)
│   └── lokol-mcp/          # Standalone MCP context refinery server (stdio JSON-RPC)
├── data/                   # Local developer test reports & run persistence (gitignored)
├── doc/
│   └── adr/                # Architecture Decision Records (ADR-0001 – ADR-0016)
├── pages/                  # GitHub Pages landing site
├── pkg/
│   ├── agent/              # Deterministic agent loop, SSE streaming & action parser
│   ├── mcp/                # Pure-Go MCP JSON-RPC stdio protocol server
│   ├── model/              # Hardware sizing & VRAM-tier model selection matrix
│   ├── probe/              # Pure Go hardware & GPU capability prober
│   ├── setup/              # Environment auditing, dependency bootstrap & weight verifier
│   ├── tools/
│   │   └── refinery/       # Context refinery: bounded reads, AST outlines, test filtering
│   ├── tui/                # Interactive Bubble Tea terminal UI with live context HUD
│   ├── update/             # Release updater: GitHub release fetching, semver targets, asset extraction
│   └── version/            # Build-time version metadata (injected via ldflags)
├── test/                   # 10-tier evaluation benchmark, Podman sandbox & Laya bridge
├── tools/
│   └── laya/               # Standalone Laya semantic evaluator script (managed via uv)
├── go.mod
└── README.md
```

## Quick Start

### Installation

Install `lokol` into your user home directory conforming to the XDG Base Directory specification (`~/.local/bin`, `~/.local/share/lokol`, `~/.config/lokol`):

```bash
curl -fsSL https://raw.githubusercontent.com/boggycreek/lokol/main/install.sh | bash
```

Customization flags can be passed to the installer:
```bash
# Custom binary directory or specific release version
curl -fsSL https://raw.githubusercontent.com/boggycreek/lokol/main/install.sh | bash -s -- --bin-dir /usr/local/bin --version v0.1.0-alpha.1

# Force build from source
curl -fsSL https://raw.githubusercontent.com/boggycreek/lokol/main/install.sh | bash -s -- --build-from-source
```

Upon installation, `install.sh` automatically invokes `lokol setup`, which performs a hardware probe, checks for `llama` / `llama-server` on PATH, verifies optimal model weights, and tests `llama-server` connectivity.

### Manual Build
```bash
make build
```

### Environment Setup & Dependency Verification
```bash
# Run environment setup & dependency audit
./bin/lokol setup

# Automatically download recommended model weights if missing
./bin/lokol setup --download-model

# Automatically download/build llama.cpp and llama-server if missing
./bin/lokol setup --install-llama

# Or use the standalone multi-OS installer directly
./install-llama.sh
```

### Probe Host Hardware
```bash
# Probe current machine (e.g. RTX 3060 12GB)
./bin/lokol probe

# Simulate running on a ThinkPad X1 Extreme (4GB VRAM)
./bin/lokol probe --simulate-vram-gib=4.0
```

### Run Autonomous Task
```bash
./bin/lokol -p "Run the tests and inspect the repository"
```

### Integration Evaluation Suite & Semantic Scoring

Verify the agent loop, tool execution, and oscillation prevention against your local model:

```bash
# Run the 10-tier live evaluation suite against llama-server (http://127.0.0.1:8080)
go test -v ./test -run TestEvalSuite_LiveEngine

# Run loop circuit breaker and oscillation intervention tests
go test -v ./test -run TestRunner_LoopCircuitBreaker

# Check Laya non-autoregressive decision model health
uv run --python .venv tools/laya/judge.py --health
```

The 10-tier benchmark exercises atomic file operations, directory grounding, bug fixes, bounded window inspection, outline parsing, git workflows, multi-turn feature addition, large-file edits, and subjective architectural synthesis scored in ~33ms via Convai's Laya model ([ADR-0014](doc/adr/0014-non-autoregressive-decision-model-judging.md)). Benchmark results persist automatically to `data/eval_results.json`.

### In-Repo MCP Context Refinery (`lokol-mcp`)
`lokol-mcp` is a standalone Model Context Protocol (MCP) server that exposes mechanical noise filtering over standard JSON-RPC stdio (per [ADR-0012](doc/adr/0012-in-repo-mcp-facades.md)):

```bash
# Run standalone MCP server
./bin/lokol-mcp

# Print version
./bin/lokol-mcp --version
```

**Exposed MCP Tools**:
- `read_outline`: Extracts high-level symbol declarations (types, interfaces, functions) without dumping inner function bodies.
- `read_window`: Bounded line reading (up to 120 lines) to defend the KV cache.
- `test_verifier` / `run_test`: Runs test suites while stripping verbose passing outputs and stack traces down to clean, actionable failure assertions.


### Self-Update & Release Management
```bash
# Update to latest stable release
lokol update

# Include unstable pre-releases / alpha builds
lokol update --pre

# Update to a specific semver release
lokol update --version=v0.1.0-alpha.1

# List all available published releases from GitHub
lokol update --list
```

---

## Contributing & Governance

At this time, **lokol is maintained solely by Boggy Creek Software LLC and is not accepting external contributions, feature requests, or pull requests**.

Public pull requests and issues are disabled to maintain strict security boundaries and development velocity.

If you are using lokol, feel free to inspect and fork the code under the terms of the [MIT License](LICENSE).

For development environment setup instructions across macOS, Debian/Ubuntu (APT), and Fedora/RHEL (RPM), see [CONTRIBUTING.md](CONTRIBUTING.md) and [`setup.sh`](setup.sh).

---

## License & Acknowledgements

- **Root License**: This project is licensed under the terms of the [MIT License](LICENSE). Copyright &copy; 2026 Boggy Creek Software LLC.
- **Acknowledgements**: See [ACKNOWLEDGEMENTS.md](ACKNOWLEDGEMENTS.md) for human-centric recognition and appreciation of our foundational open-source pillars, upstream models, and ecosystem maintainers.
- **Third-Party Notices**: See [NOTICES.md](NOTICES.md) for full legal notices, copyright statements, and redistributable licenses for third-party dependencies.
- **Software Bill of Materials (SBOM)**: Every release publishes an official SPDX JSON SBOM (`lokol-sbom.spdx.json`) and SHA256 checksums under [GitHub Releases](https://github.com/boggycreek/lokol/releases). See [ACKNOWLEDGEMENTS.md](ACKNOWLEDGEMENTS.md#software-bill-of-materials-sbom) for verification details.

