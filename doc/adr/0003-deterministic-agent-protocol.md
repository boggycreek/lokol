# ADR 0003: Deterministic Agent Protocol vs JSON Schema Tool Calling

## Status
Accepted

## Context
Small language models (1.5B to 7B parameters) struggle significantly with standard OpenAI function calling JSON schemas. They produce unescaped quotes, trailing commas, premature closing brackets, or pre-text conversation before the JSON object, leading to frequent parsing failures in standard agent frameworks (OpenCode, LangChain, AutoGen).

In addition, formatting dozens of tool schemas in the system prompt consumes thousands of tokens, directly degrading context retention and reasoning on small models.

## Decision
1. **Grammar-Enforced Sampling**: `quik` will leverage `llama.cpp` Context-Free Grammars (GBNF) for structured tool emissions whenever strict formatting is needed.
2. **Minimal Tool Primitive Architecture (The "3-Tool Rule")**:
   `quik` exposes only three core primitives to the local model:
   - `exec_bash`: Direct shell execution for inspection, searching, and running tests.
   - `edit_file`: Targeted block replacement (`path`, `target`, `replacement`).
   - `task_finish`: Final summary and handover.
3. **Tagged Delimiters as Primary Mode**:
   Small models perform reliably with code fences and explicit tags:
   ```text
   <thought>
   Plan next step
   </thought>
   <action name="exec_bash">
   grep -rn "pattern" .
   </action>
   ```
   `quik` parses streaming tokens directly in Go, halting generation as soon as `</action>` is received, executing the command, and streaming output back into the prompt cache.

## Consequences
### Positive
- Zero JSON formatting hallucinations.
- Maximum generation throughput without bloated schema token consumption.
- Rapid execution loops (<100ms turn-around between tool call and execution).
