# Bolt's Journal - Critical Learnings Only

This journal contains only critical learnings, performance bottlenecks, and non-obvious performance characteristics discovered in this codebase.

## 2026-10-24 - Precompiled AST Outline Regexes in Refinery
**Learning:** `refinery.ReadOutline` was compiling four language symbol regexes on every call, allocating ~37 KB and ~290 heap objects per invocation. Hoisting regex compilation to package-level variables reduced outline latency by ~3.5x (~66 μs to ~18 μs) and cut allocations by ~87%.
**Action:** Always precompile static `regexp.MustCompile` definitions at package level instead of inline in hot path tool executors.

## 2026-11-10 - Regex Cache Memoization for XML Tag Extraction in Regulator
**Learning:** `regulator.ExtractTagContent` was compiling a dynamic regex with `regexp.MustCompile` on every call because the tag name was variable. This caused ~9.1 μs latency and 47 heap allocations (5.3 KB) per extraction across the security gatekeeper and action boundary validator. Memoizing the compiled regexes in a concurrent `sync.Map` cache reduced latency ~7x (~9.1 μs to ~1.35 μs) and cut heap allocations by 99.4% (from 5,366 B/op to 32 B/op, 47 allocs to 1 alloc/op) while preserving exact regex parsing semantics and case preservation.
**Action:** When regular expressions are dynamic but drawn from a small discrete set of tag identifiers, use a thread-safe memoized regex cache instead of re-compiling per call or risking handwritten parser regressions.
