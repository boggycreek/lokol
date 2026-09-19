# quik

`quik` is a high-performance, local-first coding agent engine engineered to run directly against consumer GPUs without bulky runtime middleware or bloated JSON schema tool harnesses.

## Core Philosophy
1. **Zero Intermediate Abstractions**: Bypass multi-layered Node.js/Python agent frameworks in favor of a lean Go binary and managed `llama-server` process.
2. **Hardware-Aware Tiering**: Automatically probe local hardware (CPU vector extensions, VRAM, system memory) and configure the exact right model, KV cache quantization, and context window.
3. **Constrained Hardware Support**: First-class support for constrained laptops such as the Lenovo ThinkPad X1 Extreme Gen 1 (GTX 1650 Max-Q 4GB VRAM) via 3B Q4 models with pure-VRAM KV caching.
4. **Deterministic Agent Protocol**: Avoid JSON schema parsing breakdowns on small models by using tagged delimiters (`<action>`) and GBNF grammars.

## Project Structure
```
quik/
├── bin/              # Compiled quik binaries
├── cmd/
│   └── quik/         # CLI entrypoint
├── doc/
│   └── adr/          # Architecture Decision Records
├── pkg/
│   ├── agent/        # Deterministic agent loop & token streaming
│   ├── engine/       # llama-server process supervisor & manager
│   ├── hf/           # Hugging Face chunked model downloader
│   ├── model/        # Hardware sizing & model selection matrix
│   └── probe/        # Pure Go hardware & GPU capability prober
├── go.mod
└── README.md
```

## Quick Start

### Build
```bash
go build -o bin/quik ./cmd/quik
```

### Probe Host Hardware
```bash
# Probe current machine (e.g. RTX 3060 12GB)
./bin/quik probe

# Simulate running on a ThinkPad X1 Extreme (4GB VRAM)
./bin/quik probe --simulate-vram-gib=4.0
```
