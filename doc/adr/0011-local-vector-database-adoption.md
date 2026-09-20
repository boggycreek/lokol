# ADR 0011: Local Vector Database Adoption for Memory and Source Code Indexing

## Status
Accepted

## Date
2026-09-20

## Context
Persistent agent memory and large codebase comprehension both require fast similarity search:
1. **Memory Retrieval**: Finding relevant historical facts, user instructions, and past problem resolutions out of thousands of stored notes without dumping them all into the context window.
2. **Source Code Retrieval**: Finding relevant symbol definitions, struct layouts, and function signatures in an agentic coding session without conducting brute-force string greps across entire repositories.

Traditional relational stores and flat keyword searches fail to capture semantic relationships (e.g. searching for "auth handler" when the method is named `verify_bearer_token`). 

We need an embedded or local vector search engine that runs entirely on-device without cloud API dependencies.

## Decision
We adopt an **embedded, local vector database** engine (evaluating embedded **Qdrant** or **sqlite-vec** as primary candidates):

1. **Local-First & Private**: The vector database runs 100% locally on the developer machine with zero outbound network calls.
2. **Dual Collection Partitioning**:
   - `memory_collection`: Stores semantic embeddings of persistent conversational takeaways, user preferences, and cross-session learnings.
   - `code_collection`: Stores chunked AST and signature embeddings for the active workspace, enabling sub-second symbol search.
3. **Local Embedding Generation**:
   - Embeddings are generated using local embedding models (such as `nomic-embed-text-v1.5` running on CPU or through local llama-server on port 8081).
4. **Storage Location**:
   - Vector database index files reside in `${XDG_DATA_HOME:-~/.local/share}/lokol/memory/` per [ADR-0010](0010-xdg-persistent-memory-storage.md).

## Consequences

### Positive
- **High-Speed Semantic Search**: Sub-millisecond vector similarity queries for both natural language memories and source code symbols.
- **Context Preservation**: Ingests only the top 3–5 most relevant memory or code snippets (<500 tokens), preventing context window saturation.
- **Zero External Telemetry**: 100% privacy and offline functionality.

### Negative / Trade-offs
- Requires generating vector embeddings locally, which consumes modest CPU cycles during initial indexing.
- Vector indices consume disk storage proportional to codebase size and memory entries.
