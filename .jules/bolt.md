# Bolt's Journal - Critical Learnings Only

This journal contains only critical learnings, performance bottlenecks, and non-obvious performance characteristics discovered in this codebase.

## 2026-10-24 - Precompiled AST Outline Regexes in Refinery
**Learning:** `refinery.ReadOutline` was compiling four language symbol regexes on every call, allocating ~37 KB and ~290 heap objects per invocation. Hoisting regex compilation to package-level variables reduced outline latency by ~3.5x (~66 μs to ~18 μs) and cut allocations by ~87%.
**Action:** Always precompile static `regexp.MustCompile` definitions at package level instead of inline in hot path tool executors.

## 2026-10-28 - Zero-Allocation XML Tag Parsing in Regulator Boundary Check
**Learning:** `regulator.ExtractTagContent` compiled a dynamic regex pattern with `regexp.MustCompile` on every action candidate verification during the pre-execution gating pipeline, consuming ~5.3 KB and 47 heap allocations per check. Replacing the per-call regex with fast byte/string slicing eliminated all 47 allocations (down to 0 B/op) and accelerated tag extraction throughput by ~50x (~8.6 μs to ~150 ns).
**Action:** In security gating pipelines and hot-path string extractors, avoid invoking `regexp.MustCompile` per call; prefer direct index searching (`strings.Index`) or precompiled regexes to keep gating overhead zero-allocation.
