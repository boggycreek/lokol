# Bolt's Journal - Critical Learnings Only

This journal contains only critical learnings, performance bottlenecks, and non-obvious performance characteristics discovered in this codebase.

## 2026-10-24 - Precompiled AST Outline Regexes in Refinery
**Learning:** `refinery.ReadOutline` was compiling four language symbol regexes on every call, allocating ~37 KB and ~290 heap objects per invocation. Hoisting regex compilation to package-level variables reduced outline latency by ~3.5x (~66 μs to ~18 μs) and cut allocations by ~87%.
**Action:** Always precompile static `regexp.MustCompile` definitions at package level instead of inline in hot path tool executors.

## 2026-11-06 - Dynamic Regexp In Tag Extraction Hot Paths
**Learning:** `regulator.ExtractTagContent` dynamically compiled a case-insensitive tag extraction regexp (`(?si)<...`) on every invocation, adding ~8.9 µs and 47 heap allocations per call across agent action validation, audit trails, and TUI events. Precompiling the common `<path>` regex and caching compiled tag regexes via a concurrent sync.Map preserves exact regex semantic behavior (document order, Unicode boundaries, and attributes) while slashing latency by ~8x (~8.9 µs to ~1.1 µs) and cutting allocations by ~99% (from 47 allocs/op to 1 alloc/op).
**Action:** In tool validation and permission pipelines executing per-turn, avoid inline `regexp.MustCompile` definitions; precompile standard tag regexes and cache dynamic variants safely.
