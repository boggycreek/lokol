# lokol

`lokol` is a high-performance, local-first autonomous coding agent engine engineered to run directly against consumer GPUs without bulky runtime middleware or bloated JSON schema tool harnesses.

## Core Philosophy
1. **Zero Intermediate Abstractions**: Bypass multi-layered Node.js/Python agent frameworks in favor of a lean Go binary and managed `llama-server` process.
2. **Hardware-Aware Tiering**: Automatically probe local hardware (CPU vector extensions, VRAM, system memory) and configure the exact right model, single-slot dedicated engine (`-np 1`), KV cache quantization, and context window.
3. **Constrained Hardware Support**: First-class support for constrained laptops such as the Lenovo ThinkPad X1 Extreme Gen 1 (GTX 1650 Max-Q 4GB VRAM) via 3B Q4 models with pure-VRAM Q4_0 KV caching.
4. **Deterministic Agent Protocol**: Avoid JSON schema parsing breakdowns on small models by using tagged delimiters (`<action>`) and native Go mechanical tooling (`replace_file`, `write_file`, `exec_bash`).

## Project Structure
```
lokol/
├── bin/              # Compiled lokol binaries
├── cmd/
│   └── lokol/        # CLI entrypoint
├── doc/
│   └── adr/          # Architecture Decision Records
├── pkg/
│   ├── agent/        # Deterministic agent loop & token streaming
│   ├── engine/       # llama-server process supervisor & manager
│   ├── hf/           # Hugging Face chunked model downloader
│   ├── model/        # Hardware sizing & model selection matrix
│   ├── probe/        # Pure Go hardware & GPU capability prober
│   └── tui/          # Interactive Bubble Tea terminal UI with live context HUD
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

Upon installation, `install.sh` automatically invokes `lokol setup`, which performs a hardware probe, checks inference engine dependencies (`llama` / `llama-server`), verifies optimal model weights, and tests engine connectivity.

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

