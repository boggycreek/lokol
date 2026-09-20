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

# 2. Formulate the self-improvement task contract
TASK_PROMPT="TASK: In pkg/tui/tui.go, update the Welcome header message in the New() function.
Use <action name=\"replace_file\"> to update the initialText line:
<path>pkg/tui/tui.go</path>
<target>
	initialText := \"Welcome to quik. Deterministic, local-first coding agent engine.\nType your request below and press Enter to begin.\n\n\"
</target>
<replacement>
	initialText := \"⚡ Welcome to quik. High-performance, local-first autonomous coding engine.\nType your request below and press Enter to begin.\n\n\"
</replacement>

Then run: go test ./test/... && go build -o bin/quik ./cmd/quik
Finally, call <action name=\"task_finish\">Welcome message updated</action>"

echo "[2/5] Dispatching task to local GPU model via 'quik -p' in YOLO mode..."
./bin/quik --max-turns=10 -p "$TASK_PROMPT"

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
