# Acknowledgements & Open Source Credits

The **lokol** project is developed by **Boggy Creek Software LLC**. We believe deeply in the open-source ethos and acknowledge that lokol is made possible by the pioneering work of open-source software creators, foundation model researchers, maintainers, and developer communities worldwide.

lokol is engineered as a local-first, privacy-respecting autonomous AI coding companion. Achieving high-performance inference, fluid terminal interaction, and robust tool execution without external cloud dependencies requires standing on the shoulders of giants.

---

## Foundational Open Source Projects & Ecosystem Pillars

We express our sincere gratitude to the following projects, models, tools, and platforms that form the foundational pillars of lokol:

### 1. Local LLM Inference & Runtime
- **[llama.cpp](https://github.com/ggerganov/llama.cpp)** (Georgi Gerganov & Contributors) — MIT License  
  *The bedrock of modern local inference. Provides the ultra-optimized C/C++ inference engine, CUDA GPU offloading, prefix prompt cache management, and single-slot server endpoints that lokol leverages for low-latency, private, on-device intelligence.*
- **[GGML](https://github.com/ggerganov/ggml)** (Georgi Gerganov & Contributors) — MIT License  
  *Provides the tensor library for machine learning that enables quantized GGUF execution across diverse hardware tiers.*

### 2. Open Foundation Models
- **[Qwen2.5-Coder Series](https://github.com/QwenLM/Qwen2.5-Coder)** (Qwen Team / Alibaba Cloud) — Apache-2.0 License  
  *Provides state-of-the-art open code intelligence. The Qwen2.5-Coder models (3B, 7B, 14B, 32B) offer unmatched instruction following, code reasoning, and multi-file project understanding within edge and desktop hardware constraints.*
- **[nomic-embed-text](https://huggingface.co/nomic-ai/nomic-embed-text-v1.5)** (Nomic AI) — Apache-2.0 License  
  *Enables high-fidelity local text and codebase embeddings for semantic retrieval.*
- **[BGE-Reranker](https://github.com/FlagOpen/FlagEmbedding)** (BAAI) — Apache-2.0 License  
  *Provides high-precision local reranking to ensure that only the most relevant code contexts enter bounded local inference windows.*

### 3. Terminal User Interface & Interaction
- **[Charm](https://charm.sh/)** (Charmbracelet, Inc.) — MIT License  
  *The modern CLI stack that powers lokol's reactive terminal user interface:*
  - **[Bubble Tea](https://github.com/charmbracelet/bubbletea)**: The Elm-architecture terminal application framework providing clean state management and event-driven rendering.
  - **[Lip Gloss](https://github.com/charmbracelet/lipgloss)**: Fluent, expressive terminal layout and style definitions.
  - **[Bubbles](https://github.com/charmbracelet/bubbles)**: Core UI components (viewports, text areas, spinners) delivering smooth interactive sessions.

### 4. Graph Backlog & Distributed Task Tracking
- **[Beads (`bd`)](https://github.com/gastownhall/beads)** (Gas Town Hall / Steve Yegge) — MIT / Apache-2.0 Licenses  
  *Provides durable, graph-based issue tracking, blocker management, and multi-session work memory for human and agent workflows.*
- **[Dolt](https://www.dolthub.com/)** (DoltHub, Inc.) — Apache-2.0 License  
  *The version-controlled relational database engine underpinning distributed, peer-to-peer Beads synchronization.*

### 5. Systems Layer & Runtime
- **[The Go Programming Language](https://go.dev/)** (The Go Authors & Google) — BSD-3-Clause License  
  *Provides the robust systems runtime, zero-allocation memory efficiency, cross-platform concurrency primitives, and static binary distribution that makes lokol fast, portable, and dependable.*

---

## Legal Notices & Third-Party Licenses

For formal legal copyright notices, exact SPDX license identifiers, and redistributable third-party license texts for all direct and transitive dependencies bundled or distributed with lokol, please consult **[NOTICES.md](file:///home/brian/Workspaces/boggycreek/lokol/NOTICES.md)**.
