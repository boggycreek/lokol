#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "laya",
# ]
# ///
"""
Laya Semantic Judge & Relative Correctness Evaluator for lokol.

Uses Convai's non-autoregressive 'System 1' decision model (ModernBERT-based)
to evaluate agent trajectories and outputs in a single forward pass (~33ms)
without chat LLM token variance or hallucinations.
"""

import argparse
import json
import sys


def get_agent():
    import laya
    # Preload default English model (convaiinnovations/laya)
    return laya.load("convaiinnovations/laya")


def evaluate(state: str, instructions: str, threshold: float = 0.5):
    agent = get_agent()

    questions = {
        "correct": {
            "type": "noul",
            "instructions": instructions,
        },
        "quality": {
            "type": "score",
            "instructions": "Rate the overall correctness and fidelity of the agent's work.",
            "criteria": ["unacceptable", "poor", "acceptable", "good", "flawless"],
        }
    }

    result = agent.predict(state, questions)
    
    # In Laya, noul returns probability of true [0.0 - 1.0]
    # result structure has answers for each question
    correct_p = 0.0
    quality_lvl = 0

    if "answers" in result:
        answers = result["answers"]
        if "correct" in answers:
            ans = answers["correct"]
            if isinstance(ans, dict):
                correct_p = ans.get("noul", ans.get("confidence", ans.get("value", ans.get("probability", 0.0))))
            elif isinstance(ans, (int, float)):
                correct_p = float(ans)
            elif isinstance(ans, bool):
                correct_p = 1.0 if ans else 0.0

        if "quality" in answers:
            q_ans = answers["quality"]
            if isinstance(q_ans, dict):
                quality_lvl = q_ans.get("score", q_ans.get("value", 0))
            elif isinstance(q_ans, (int, float)):
                quality_lvl = q_ans

    passed = correct_p >= threshold

    return {
        "passed": passed,
        "probability": correct_p,
        "quality_level": quality_lvl,
        "threshold": threshold,
        "raw": result,
    }


def main():
    parser = argparse.ArgumentParser(description="Laya Evaluator for lokol integration test harness")
    parser.add_argument("--health", action="store_true", help="Check if Laya can be loaded")
    parser.add_argument("--state", type=str, help="Context/trajectory to evaluate")
    parser.add_argument("--instructions", type=str, default="Did the agent correctly and faithfully fulfill the prompt?", help="Evaluation query")
    parser.add_argument("--threshold", type=float, default=0.5, help="Confidence threshold to pass")
    parser.add_argument("--json", action="store_true", help="Read input from JSON on stdin")

    args = parser.parse_args()

    if args.health:
        try:
            agent = get_agent()
            print(json.dumps({"status": "ok", "model": "convaiinnovations/laya"}))
            sys.exit(0)
        except Exception as e:
            print(json.dumps({"status": "error", "error": str(e)}), file=sys.stderr)
            sys.exit(1)

    state = args.state
    instructions = args.instructions
    threshold = args.threshold

    if args.json or not state:
        try:
            payload = json.load(sys.stdin)
            state = payload.get("state", state)
            instructions = payload.get("instructions", instructions)
            threshold = payload.get("threshold", threshold)
        except Exception as e:
            print(json.dumps({"error": f"Failed to parse JSON stdin: {e}"}), file=sys.stderr)
            sys.exit(1)

    if not state:
        print(json.dumps({"error": "Missing state for evaluation"}), file=sys.stderr)
        sys.exit(1)

    try:
        outcome = evaluate(state, instructions, threshold)
        print(json.dumps(outcome, indent=2))
        sys.exit(0 if outcome["passed"] else 2)
    except Exception as e:
        print(json.dumps({"error": str(e)}), file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
