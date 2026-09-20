# ADR 0001: Implementation Language and Engine Architecture

## Status
Accepted

## Context
Running local coding agents typically suffers from heavy abstraction layers (Node.js/Python frameworks, LangChain, OpenCode) that add latency, introduce schema conversion issues, and consume significant host memory. Furthermore, linking C++ engines directly into the host process via CGO or Rust FFI risks terminating the entire agent session whenever a GPU Out-of-Memory (OOM) or CUDA illegal memory access occurs.

We need a high-performance, single-binary agent CLI that is resilient, portable, easy to maintain, and cross-platform.

## Decision
1. **Language**: Use **Go** (`github.com/boggycreek/quik`). Go compiles to a single, static binary with zero external runtime dependencies, has a standard library with built-in HTTP/SSE streaming, simple concurrency primitives, and straightforward cross-compilation.
2. **Engine Architecture**: Use a **Managed Process Isolation** model for the inference engine (`llama-server`). Rather than linking `llama.cpp` directly into the Go binary (which makes the binary fragile to CUDA driver mismatches and OOM panics), `quik` will detect the host GPU, manage downloading or building an optimized `llama-server` binary, and supervise it as a child process via Unix domain socket or local loopback HTTP.
3. **Engine Supervision**: If `llama-server` terminates or faults due to VRAM pressure, `quik` catches the exit status, cleans up system resources, and provides actionable remediation without crashing the user's terminal session.

## Consequences
### Positive
- Rock-solid process boundary: Go manages the state, file system operations, and agent loop; C++/CUDA handles tensor operations.
- Clean updates: `llama-server` can be patched or recompiled with new CUDA / Vulkan / CPU flags without rebuilding the `quik` CLI binary.
- Pure Go portability: `quik` can be built with `CGO_ENABLED=0` for portable distribution.

### Negative / Trade-offs
- Requires managing binary discovery or extraction of `llama-server` on initial setup.
- Communication has minimal loopback IPC overhead (negligible compared to LLM token generation latency).
