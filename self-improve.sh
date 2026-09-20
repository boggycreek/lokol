#!/bin/bash
set -euo pipefail

# self-improve.sh: Orchestrates local GPU self-improvement of quik using quik itself.

WORKSPACE="/home/brian/Workspaces/boggycreek/quik"
cd "$WORKSPACE"

echo "=========================================================="
echo "   quik Self-Improvement Harness (Running on Local GPU)   "
echo "=========================================================="

# 1. Baseline verification
echo "[1/5] Verifying baseline tests and compiling binary..."
go test ./test/...
go build -o bin/quik ./cmd/quik

BASELINE_COMMIT=$(git rev-parse HEAD)
echo "Baseline bookmark: $BASELINE_COMMIT"

# 2. Formulate the Claude Code-like TUI UX improvement task contract
TASK_PROMPT="You are improving the TUI in pkg/tui/tui.go.
Goal: Adopt the Claude Code developer experience style.
Specifically:
1. Do NOT dump raw tool output or execution logs into the chat viewport.
2. In pkg/tui/tui.go, when actionExecutedMsg is handled, only print a clean, minimal 1-line acknowledgment:
   e.g. '✓ Executed: <command>' (or '✗ Failed: <command>' if error).
   Do NOT print 'OUTPUT:' boxes or full command output in m.appendLog.
3. Keep the full output only in toolResult inside m.history for context, not visible on the chat stream.
4. If the agent emits an explanation or question, show that cleanly.
5. Inspect pkg/tui/tui.go, edit it, and verify by running: go test ./test/... && go build -o bin/quik ./cmd/quik
6. When done, call <action name=\"task_finish\">TUI streamlined</action>."

echo "[2/5] Dispatching task to local GPU model via 'quik exec' in YOLO mode..."
./bin/quik exec --max-turns=10 "$TASK_PROMPT"

echo "[3/5] Assessing changes made by local GPU agent..."
git diff --stat

# 4. Assess regression vs advancement
echo "[4/5] Running test suite and build verification..."
if go test ./test/... && go build -o bin/quik ./cmd/quik; then
    echo "✅ Tests passed and binary built cleanly!"
    
    # Check if files were actually modified
    if git diff --quiet; then
        echo "⚠️ No changes were made by the agent."
    else
        echo "[5/5] Advancement confirmed! Committing advancement bookmark..."
        git add pkg/tui/tui.go
        git commit -m "feat(tui): streamline chat stream to Claude Code minimal ack style (written by local quik agent)"
        echo "Advancement committed: $(git rev-parse HEAD)"
    fi
else
    echo "❌ Regression detected! Tests failed or build broken."
    echo "Reverting to baseline bookmark: $BASELINE_COMMIT"
    git reset --hard "$BASELINE_COMMIT"
    exit 1
fi

echo "=========================================================="
echo "Self-improvement cycle complete!"
echo "=========================================================="
