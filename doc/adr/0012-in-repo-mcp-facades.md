# ADR 0012: In-Repo MCP Servers as Architectural Facades for Deterministic Tool Call Invocations

## Status
Accepted

## Date
2026-09-20

## Context
Running local models (e.g. 7B and 3B parameter models) as autonomous agents requires solving two interrelated problems:

1. **Tool Invocation Reliability (The Bounded Tool Surface Problem)**:
   Small local models cannot reliably emit complex, deeply nested JSON tool schemas or handle volatile third-party tool interfaces. Variations in parameter naming, nested payloads, or unescaped characters cause syntax breakdown, catastrophic looping, and parsing failures (as identified in [ADR-0003](0003-deterministic-agent-protocol.md)). The model requires an **invariant, lean, highly predictable tool calling surface** using explicit action delimiters (e.g. `<action name="read_outline">`, `<action name="remember">`, `<action name="search_code">`).

2. **Underlying Volatility & Bloat (The Tool Churn Problem)**:
   The underlying tools and backends that an agent needs (compilers, AST parsers, test runners, vector databases like Qdrant/sqlite-vec, memory stores, external APIs) change constantly and produce noisy, voluminous output. If raw tools or volatile third-party APIs are bound directly to the agent loop, either:
   - The LLM context is flooded with verbose CLI dumps (5,000 lines of test stack traces or raw grep dumps), eviction-saturating the KV cache.
   - Any change to an underlying database, library, or OS tool requires altering the prompt schema, retraining the model's generation expectations, or rewriting core agent loop logic.

We need an overarching architectural policy governing all tools and services maintained within this repository.

## Decision
We adopt the **Facade Pattern across ALL Model Context Protocol (MCP) servers** maintained within the `lokol` repository (`lokol-mcp`, `lokol-memory`, etc.):

```mermaid
flowchart TD
    subgraph AgentRuntime ["lokol Agent Runtime"]
        LLM["Local LLM (Qwen 7B / 3B / MoE)"]
        StreamParser["Action Delimiter Stream Parser\n(<action name=\"tool_name\">)"]
        MCPClient["Standard MCP Client (stdio JSON-RPC)"]
    end

    subgraph MCPFacades ["In-Repo MCP Facades (Maintained in this Repo)"]
        Refinery["lokol-mcp (Context Refinery)\nTools: read_outline, read_window, run_test"]
        MemoryFacade["lokol-memory (Memory Facade)\nTools: remember, recall, search_code"]
    end

    subgraph VolatileBackends ["Volatile / Noisy Backends (Encapsulated)"]
        AST["Go AST / ctags / tree-sitter"]
        TestHarness["go test / npm test / pytest / cargo"]
        VectorDB["Qdrant embedded / sqlite-vec / DuckDB"]
        Embedder["Local nomic-embed-text / llama-embedding"]
        FileSystem["Raw Disk / Git Workspace"]
    end

    LLM -->|Emits tool call invocations| StreamParser
    StreamParser --> MCPClient
    MCPClient <==>|Invariant MCP Protocol| Refinery
    MCPClient <==>|Invariant MCP Protocol| MemoryFacade

    Refinery -.->|Slices & filters noise| AST
    Refinery -.->|Gates error traces to 1-line| TestHarness
    Refinery -.->|Window bounds <=100 lines| FileSystem
    MemoryFacade -.->|Vector similarity & indexing| VectorDB
    MemoryFacade -.->|Embeds query| Embedder
```

### 1. Invariant Tool Call Boundary
Every MCP server authored and maintained in `lokol` exists to provide an **invariant, contract-guaranteed tooling facade** tailored specifically for small model invocation reliability:
- The agent prompt only exposes minimal, stable action names and flat, non-nested parameters.
- The LLM emits tool call invocations against this stable facade, completely insulated from how the underlying tasks are actually performed.

### 2. Radical Noise Gate & Mechanical Filtering
The in-repo MCP servers are explicitly responsible for **mechanical pre-filtering and summarization** before returning bytes to the agent loop:
- `read_outline`: Filters raw source code down to struct/function signatures, preventing the LLM from ingesting whole files.
- `read_window`: Strictly bounds reading to <= 100 lines.
- `run_test`: Intercepts raw test runner stdout; suppresses thousands of lines of passing noise and stack traces, returning only the failing assertion line.
- `recall` / `search_code`: Queries the local vector database and extracts only the top 3–5 exact chunks (<500 tokens), preventing raw vector dumps.

### 3. Encapsulation & Backend Replaceability
Because the MCP servers act as facades:
- **Backend Swapping**: We can replace `sqlite-vec` with embedded `Qdrant`, or swap Go AST parsing for tree-sitter, without changing a single line in the core agent loop or changing a single token in the LLM's system prompt.
- **Pure-Go Static Agent Binary**: The core `lokol` binary remains `CGO_ENABLED=0` static Go. Heavy C/C++ or Rust dependencies (like vector search or tree-sitter) are isolated inside their respective MCP server child processes.
- **Multi-Agent Interoperability**: These in-repo MCP facades can be plugged seamlessly into external orchestrators (Claude Code, Cursor, `agent-sandbox` fleet).

## Consequences

### Positive
- **Guaranteed Tool Invocation Reliability**: The local 7B/3B LLM targets an invariant, minimal set of action delimiters and tool signatures that it has been calibrated to generate deterministically.
- **KV Cache Defense**: Strict mechanical filtering at the MCP layer prevents context bloat from verbose OS commands.
- **Total Decoupling**: Underlying libraries, compilers, and vector databases can be upgraded or replaced without breaking prompt contracts or agent control flow.

### Negative / Trade-offs
- Each capability domain requires maintaining its companion MCP facade binary (`lokol-mcp`, `lokol-memory`).
- Communication introduces minor loopback stdio JSON-RPC IPC overhead (<1ms, negligible compared to LLM generation).
