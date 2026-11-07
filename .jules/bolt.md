# Bolt's Journal - Critical Learnings Only

This journal contains only critical learnings, performance bottlenecks, and non-obvious performance characteristics discovered in this codebase.

## 2026-10-24 - Precompiled AST Outline Regexes in Refinery
**Learning:** `refinery.ReadOutline` was compiling four language symbol regexes on every call, allocating ~37 KB and ~290 heap objects per invocation. Hoisting regex compilation to package-level variables reduced outline latency by ~3.5x (~66 μs to ~18 μs) and cut allocations by ~87%.
**Action:** Always precompile static `regexp.MustCompile` definitions at package level instead of inline in hot path tool executors.

## 2026-10-24 - Zero-Regex XML Tag Content Extraction in Regulator
**Learning:** `regulator.ExtractTagContent` was compiling a dynamic regex with `regexp.MustCompile` on every single invocation across boundary checks, rejection tracking, agent loop dispatch, and IPC routing. Replacing runtime regex compilation with zero-regex string index searching eliminated ~30 KB and hundreds of heap allocations per call, dropping allocation to 0 B/op and latency to ~234 ns.
**Action:** Use direct string scanning (`strings.Index`) rather than dynamically compiled regular expressions for XML parameter extraction on security and IPC hot paths.
