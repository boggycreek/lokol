# Spike Findings: Streaming XML Tokenizer vs. Regex Action Parsing (lokol-f78.2)

**Date**: 2026-10-02  
**Author**: Lokol Autonomous Systems / Brian Posey  
**Status**: Completed  
**Associated Beads**:
- `lokol-f78`: (EPIC) Implementation Quality: Address Code Naming, Robustness, and Missing Capabilities
- `lokol-f78.1`: Add regression test cases for regex action parser edge cases (Completed)
- `lokol-f78.2`: Spike: Evaluate streaming XML tokenizer to replace regex action parsing (This Spike)

---

## 1. Executive Summary & Problem Formulation

In Lokol's agentic loop (`liblokol/agent`), the model expresses actions (tool calls) via structured tags embedded in plain-text generation:

```xml
<action name="write_file">
{"path": "hello.txt", "content": "world"}
</action>
```

Currently, action extraction is implemented in `liblokol/agent/client.go` using a regular expression:
```go
actionRegex = regexp.MustCompile(`(?s)(?:^|\n)[\s\x60]*<action\s+name=["']?([a-zA-Z0-9_-]+)["']?\s*>(.*?)(?:</action>|` + "```" + `|$)`)
```

While this regex handles standard turns and permissive boundaries (such as unquoted attribute names, trailing markdown fences, or EOF termination), regex parsing of hierarchical markup is fundamentally constrained. As tool payloads become more complex—incorporating raw bash scripts with shell redirections (`cat <<EOF > out.xml`), Go generics (`Map[K, V]()`), HTML/JSX templates, or nested XML tags—the regex parser risks false positive truncations or extraction anomalies.

This spike evaluates whether Lokol should transition from the current regex parser to:
1. Go's standard library streaming XML decoder (`encoding/xml.Decoder`), or
2. A custom deterministic finite-state automaton (FSM) streaming lexer.

---

## 2. Evaluation of Alternatives

### Alternative A: Current Regex Action Parser

#### Mechanics
Scans the text using a single regular expression with DOTALL mode (`(?s)`). Extracts `name` via capture group 1 and payload via non-greedy capture group 2 (`(.*?)`), terminating on `</action>`, closing markdown backticks, or end-of-string.

#### Strengths
- **Simplicity**: Zero boilerplate; single function call.
- **Lenient Attribute Syntax**: Handles missing quotes (`<action name=exec_bash>`) and varied whitespace.
- **Tolerance for Malformed Markup**: Does not enforce XML well-formedness rules. If the inner payload contains raw unescaped `<` or `>` characters (common in source code and bash pipes), regex passes them through verbatim.
- **Graceful EOF Handling**: If the LLM runs out of tokens before writing `</action>`, the regex still extracts the partial payload up to `$`.

#### Vulnerabilities & Edge Cases
1. **Premature Termination on Literal Tag**: If a bash script or code file being written contains the literal string `</action>`, the non-greedy match `(.*?)` stops immediately at the first occurrence, truncating the code payload.
2. **Greedy Trailing Boundary Overshoot**: In multi-action outputs or outputs with trailing markdown blocks, imperfect greedy/lazy boundaries can misattribute delimiters.
3. **No Streaming Tokenization**: The regex requires the entire assistant response to be buffered in memory before execution can begin.

---

### Alternative B: Go Standard Library `encoding/xml.Decoder`

#### Mechanics
`encoding/xml.Decoder` reads tokens (`xml.StartElement`, `xml.EndElement`, `xml.CharData`, `xml.Comment`) sequentially from an `io.Reader`.

```go
decoder := xml.NewDecoder(strings.NewReader(response))
for {
    token, err := decoder.Token()
    if err != nil { break }
    switch t := token.(type) {
    case xml.StartElement:
        if t.Name.Local == "action" {
            // Read until matching EndElement
        }
    }
}
```

#### Strengths
- **True Hierarchy & Nesting**: Properly tracks depth counts. Nested tags like `<diff><file>foo</file></diff>` do not prematurely close the outer `<action>` element.
- **Streaming Native**: Can parse directly off an incoming chunk stream without buffering entire responses.
- **Standard Library**: No external dependencies; thoroughly hardened against buffer overruns and recursion depth exploits.

#### Fatal Flaws with Raw LLM Outputs
1. **Strict XML Well-Formedness Enforcement**:
   Go's `encoding/xml` is a standard, conforming XML 1.0 parser. It **strictly forbids** bare `<` and `&` characters in text nodes:
   - A shell command like `<action name="exec_bash">cat < file.txt & ls</action>` immediately returns:
     `XML syntax error on line 1: invalid character '<' in input content` or `invalid character '&'`.
   - Code writing tools generating Go code with generics (`type Queue[T any] struct`) or C++ (`std::vector<int>`) fail with parse errors.
2. **Attribute Quoting Inflexibility**:
   `encoding/xml` strictly requires quoted attributes (`name="foo"`). Models frequently output unquoted attributes (`name=write_file`), which causes `xml.Decoder` to fail immediately.
3. **Preamble Rejection**:
   LLMs typically output conversational conversational preamble before the `<action>` tag. While `xml.Decoder` skips leading whitespace, arbitrary conversational text containing angle brackets crashes the decoder before it ever reaches `<action>`.

**Verdict on `encoding/xml`**: Conforming XML parsers are **unsuitable** for parsing LLM conversational output without an extensive and complex pre-sanitization pass (which itself would re-introduce the very fragility we seek to eliminate).

---

### Alternative C: Custom Deterministic Finite-State Automaton (FSM Lexer)

#### Mechanics
A dedicated lightweight state machine that scans a token or rune stream through five distinct states:
1. `StateText`: Searching for literal opening marker `<action`.
2. `StateTag`: Parsing the `name` attribute (tolerant of unquoted names and arbitrary whitespace).
3. `StateTagClose`: Finding the closing `>` of the opening `<action>` tag.
4. `StatePayload`: Accumulating bytes into the action payload. In this state, **all characters including `<`, `>`, `&`, and nested XML tags are treated as raw literals**. Only the exact sequence `</action>` triggers state transition.
5. `StateComplete`: Emitting the extracted action.

```
 [StateText] --- (find "<action") ---> [StateTag]
                                           |
                                 (extract name attr)
                                           |
                                           v
 [StateComplete] <-- (find "</action>") -- [StatePayload]
```

#### Strengths
- **Verbatim Inner Character Transparency**: Allows unescaped `<file.txt`, `&`, Go generics, and nested HTML/XML inside the payload without tripping XML syntax errors.
- **Nested Tag Disambiguation**: Can track balanced tag depth if nested `<action>` blocks ever need to be supported, or use escape sequence markers.
- **Zero Allocations & True Streaming**: Can process incoming token chunks from `llama-server` SSE streams in real time. Can begin unmarshaling JSON arguments while the model is still generating.
- **Lenient Grammar**: Effortlessly supports unquoted attributes (`name=write_file`), single quotes, double quotes, and unclosed tags at EOF.

---

## 3. Comparison Matrix

| Criterion | A. Regex Parser (Current) | B. `encoding/xml.Decoder` | C. Custom Streaming FSM |
| :--- | :---: | :---: | :---: |
| **Nested `<action>` handling** | ⚠️ Fragile (stops at 1st `</action>`) | ✅ Full depth tracking | ✅ Configurable depth tracking |
| **Raw `<` / `>` in payload (generics, pipes)** | ✅ Supported | ❌ **Fatal Failure** (XML syntax error) | ✅ Supported |
| **Unquoted attributes (`name=tool`)** | ✅ Supported | ❌ Fatal Failure | ✅ Supported |
| **Conversational preamble tolerance** | ✅ Supported | ⚠️ Partial (trips on bare `<`) | ✅ Supported |
| **Streaming token-by-token evaluation** | ❌ Full buffer required | ✅ Streamable | ✅ Streamable |
| **Unclosed tag at EOF tolerance** | ✅ Supported | ❌ Error | ✅ Supported |
| **Allocation & CPU Overhead** | Low (regex engine) | Low | **Optimal** (zero-alloc ring/slice) |
| **Maintenance Complexity** | Minimal (~20 lines) | Medium | Moderate (~120 lines) |

---

## 4. Key Findings & Empirical Observations

1. **`encoding/xml` is a Non-Starter for LLM Tool Execution**:
   Because LLMs generate programming language syntax (Go generics, C++ templates, bash pipes, bitwise operators) directly inside tool call arguments, an XML parser that enforces XML 1.0 conformance causes frequent, unrecoverable failures on valid code generation turns.

2. **Current Regex is Proven and Sufficient for Synchronous Loop**:
   With the 20+ regression test cases added in `lokol-f78.1` (covering unquoted names, single/double quotes, trailing markdown fences, JSON fallback, missing closing tags, and nested non-action tags), the current regex parser is robust for all current synchronous agent scenarios.

3. **When an FSM is Warranted**:
   The transition to an FSM parser (Alternative C) becomes strictly necessary when:
   - Lokol moves to **streaming tool execution** (starting tool preparation while the LLM is still streaming argument tokens).
   - Tools accept nested sub-actions or code editing payloads that frequently contain the exact literal `</action>` delimiter.

---

## 5. Architectural Recommendation & Action Plan

1. **Retain Regex for v1.x Core Engine**:
   Keep the regex action parser as the primary extraction mechanism in `liblokol/agent/client.go`. The test suite in `liblokol/agent/agent_test.go` (`TestParseAction`) now guarantees stability against known edge cases.
2. **Standardize Escaping Protocol for Code Payloads**:
   If an agent needs to write source code containing `</action>`, tools should instruct the model or format the action payload inside CDATA or standard JSON escaping (`\u003c/action\u003e`), both of which pass through the current regex parser cleanly.
3. **Future Bead (`lokol-stream-fsm`)**:
   When real-time token streaming tool execution is scheduled, implement Alternative C (`ActionScanner`) as an `io.Writer` / streaming chunk consumer.
