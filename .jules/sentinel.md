# Sentinel Security Journal

## 2026-11-06 - Shell Regulator Static Reverse Shell Detection & False Positive Evasions
**Vulnerability:** Shell execution in autonomous mode (`exec_bash`) was vulnerable to interactive reverse shells and socket redirections (`/dev/tcp/...`, `/dev/udp/...`, `nc -e /bin/sh`, and piped netcat shells) because deterministic static inspection lacked patterns for network socket manipulation. Additionally, initial attempts at regex pattern matching suffered from:
1. Substring false positives where flags like `-e` or `-c` matched inside hostnames and parameters (e.g. `test-cluster`, `host-example.org`, `app-client`).
2. Ambient shell execution bypasses where `/dev/tcp/` redirects were executed directly without an explicit preceding `bash` invocation (`exec 5<>/dev/tcp/...`), since `exec_bash` already executes commands within an ambient bash process.
3. Pipe-based reverse shells (`nc host port | /bin/sh` or FIFO redirection) that omit `-e`/`-c` execution flags entirely.

**Learning:** Static regex-based security controls on shell commands must account for shell interpreter context (ambient subshell execution) and enforce strict word/flag boundaries (`\s+(?:-[a-z0-9]*[ec]\b|--(?:sh-)?exec\b)`). Regex matching on virtual shell pseudo-devices like `/dev/tcp/host/port` should not assume a prefixed interpreter command because the runtime shell interpreter is already ambient.

**Prevention:** Always test static regex danger rules against both adversarial bypass permutations (e.g., ambient redirects, pipe chaining) and common benign arguments (hostnames containing flag characters like `cluster` or `example`) to ensure zero false-positive disruption to developer workflows while maintaining complete risk coverage.

## 2026-11-06 - Shell Regulator Bypass via Interpreter Verification Tools (`run_test`)
**Vulnerability:** The static shell inspection pipeline (`NewStaticShellStage`) strictly filtered actions by `action.Name == "exec_bash"`. However, the specialized test tool `run_test` also executed arbitrary shell commands directly via `bash -c` in `refinery.RunTestVerifier`. As a result, critical destructive commands (e.g. `rm -rf /`) or reverse shell connections (e.g. `nc -e /bin/sh ...`) submitted as a `run_test` action bypassed pre-execution static shell regulation entirely, as well as defense-in-depth checks in `builtinRunTest`.

**Learning:** When securing agentic tools that invoke shell interpreters or subshells, static security gating must not assume only the primary shell tool (`exec_bash`) executes shell strings. Every action that invokes an underlying shell interpreter (such as `run_test`) must be bound to the static risk regulator pipeline.

**Prevention:** Ensure all tools that execute shell commands pass through `InspectShellRisk` in both the regulator pipeline stage and individual builtin tool handlers.
