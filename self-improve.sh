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
TASK_PROMPT="TASK: Improve the TUI in pkg/tui/tui.go to adopt the Claude Code style.
Problem: When an action finishes, tui.go prints the raw command output into the chat viewport:
		box := outputBoxStyle.Render(fmt.Sprintf(\"OUTPUT:\n%s\", output))
		m.appendLog(box + \"\n\")
Goal: Replace those 2 lines in pkg/tui/tui.go with a clean 1-line acknowledgment:
		m.appendLog(\"✓ Executed successfully\n\")

Run this python command to apply the edit:
python3 -c '
with open(\"pkg/tui/tui.go\", \"r\") as f:
    lines = f.readlines()
new_lines = []
for line in lines:
    if \"box := outputBoxStyle.Render\" in line:
        continue
    elif \"m.appendLog(box + \" in line:
        new_lines.append(\"\t\tm.appendLog(\\\"✓ Executed successfully\\\\n\\\")\n\")
    else:
        new_lines.append(line)
with open(\"pkg/tui/tui.go\", \"w\") as f:
    f.writelines(new_lines)
print(\"UPDATED_TUI\")
'

Then run: go test ./test/... && go build -o bin/quik ./cmd/quik
Finally, call <action name=\"task_finish\">TUI streamlined</action>"

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
