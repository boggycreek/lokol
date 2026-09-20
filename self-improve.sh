#!/bin/bash
# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

set -euo pipefail

# self-improve.sh: Orchestrates local GPU self-improvement of lokol using lokol itself.

WORKSPACE="/home/brian/Workspaces/boggycreek/lokol"
if [ ! -d "$WORKSPACE" ]; then
    WORKSPACE="/home/brian/Workspaces/boggycreek/quik"
fi
cd "$WORKSPACE"

echo "=========================================================="
echo "   lokol Self-Improvement Harness (Running on Local GPU)  "
echo "=========================================================="

# 1. Baseline verification
echo "[1/5] Verifying baseline tests and compiling binary..."
go test ./test/...
go build -o bin/lokol ./cmd/lokol

BASELINE_COMMIT=$(git rev-parse HEAD)
echo "Baseline bookmark: $BASELINE_COMMIT"

# 2. Formulate the self-improvement task contract
TASK_PROMPT="TASK: In README.md, ensure all references are updated to lokol.
Then run: go test ./test/... && go build -o bin/lokol ./cmd/lokol
Finally, call <action name=\"task_finish\">All tests passing and binary verified</action>"

echo "[2/5] Dispatching task to local GPU model via 'lokol -p' in YOLO mode..."
./bin/lokol --max-turns=10 -p "$TASK_PROMPT"

echo "[3/5] Assessing changes made by local GPU agent..."
git diff --stat

# 4. Assess regression vs advancement
echo "[4/5] Running test suite and build verification..."
if go test ./test/... && go build -o bin/lokol ./cmd/lokol; then
    echo "✅ Tests passed and binary built cleanly!"
    
    # Check if files were actually modified
    if git diff --quiet; then
        echo "⚠️ No changes were made by the agent."
    else
        echo "[5/5] Advancement confirmed! Committing advancement bookmark..."
        git add -u
        git commit -m "chore(lokol): automated self-improvement update (written by local lokol agent)"
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
