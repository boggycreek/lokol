# Bolt's Journal - Critical Learnings Only

This journal contains only critical learnings, performance bottlenecks, and non-obvious performance characteristics discovered in this codebase.

## 2026-10-24 - Precompiled AST Outline Regexes in Refinery
**Learning:** `refinery.ReadOutline` was compiling four language symbol regexes on every call, allocating ~37 KB and ~290 heap objects per invocation. Hoisting regex compilation to package-level variables reduced outline latency by ~3.5x (~66 μs to ~18 μs) and cut allocations by ~87%.
**Action:** Always precompile static `regexp.MustCompile` definitions at package level instead of inline in hot path tool executors.

## 2026-11-06 - Dynamic Regex Compilation in Hot-Path XML Tag Parsing
**Learning:** `regulator.ExtractTagContent` dynamically compiled a regex with `regexp.MustCompile` on every call because the XML tag name was variable. This caused ~8.2 μs latency and 47 heap allocations (5.3 KB) per tag extraction on the hot path for all action checks. Replacing regex compilation with direct byte/string scanning reduced latency by ~93x (~8,200 ns to ~88 ns) and completely eliminated heap allocations (0 B/op, 0 allocs/op).
**Action:** Replace dynamic per-call regex compilation in tag/token extraction hot paths with direct byte/string scanning.
