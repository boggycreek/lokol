# lokol Core Session Driver (`tools/core_driver`)

`core_driver` is an in-process Go test harness and interactive session driver that executes directly against the [`agent.SessionCore`](../../liblokol/agent/session.go) interface.

## Architectural Purpose

Unlike black-box test harnesses (`tools/cli_harness` or `cmd/lk`), `core_driver` bypasses terminal rendering, stdout/stderr string scraping, and subprocess startup overhead to drive the agent engine directly in Go memory:

1. **Direct In-Process Execution**: Instantiates [`agent.Session`](../../liblokol/agent/session.go) directly in-process, streaming turn tokens directly into Go channels.
2. **Structured Event Assertions**: Directly parses and executes [`agent.Action`](../../liblokol/agent/client.go) structs with microsecond latency.
3. **Repeatable Scenario Probing**: Reads version-controlled JSONL scenarios (`tools/scenarios/scenarios.jsonl`) and executes them with configurable repetition counts (`-r N`) to evaluate determinism and inference cache behavior.
4. **Semantic Scoring**: Automatically bridges to Convai's Laya decision model (`tools/laya/judge.py`) for semantic evaluation and scoring.
5. **Interactive REPL Prompting**: Provides an interactive developer prompt loop directly into the core engine with real-time slot telemetry and mode switching.

---

## Usage

### 1. Run Repeatable Scenarios

```bash
# Run all capability probe scenarios
go run ./tools/core_driver

# Run a specific scenario by ID
go run ./tools/core_driver -s probe_gpu_grounding

# Run a scenario 3 times to test repeatability & cache stability
go run ./tools/core_driver -s probe_gpu_grounding -r 3

# Run with verbose token streaming
go run ./tools/core_driver -s probe_directory_recovery -v

# Run with automated Beads bug reporting for failures
go run ./tools/core_driver --file-beads
```

### 2. Interactive Session REPL

```bash
# Start an interactive session directly on the core engine
go run ./tools/core_driver -i

# Start in coding mode
go run ./tools/core_driver -i -m coding
```

#### In-Session Commands
- `/mode <general|coding|moe>`: Dynamically switch persona mode.
- `/slot`: Inspect real-time engine slot context and token metrics.
- `/history`: Print full conversation history.
- `/reset`: Clear history back to mode system prompt.
- `/clear`: Clear terminal screen.
- `/quit` or `/exit`: Exit session.

---

## Quality Gates

```bash
# Run unit tests
make -C tools/core_driver test

# Build binary
make -C tools/core_driver build
```
