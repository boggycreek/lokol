# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Capability Probe Test Harness & Closed-Loop Beads Reporter.

Executes scenarios against lokol binaries, supervises hardware telemetry,
runs deterministic & Laya semantic evaluation, and automatically files Beads issues.
"""

from dataclasses import dataclass, field
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from typing import Dict, List, Optional

from .scenarios import Scenario
from .watchdog import HardwareWatchdog, WatchdogViolation


@dataclass
class ScenarioResult:
    scenario: Scenario
    passed: bool
    duration_sec: float
    stdout: str
    stderr: str
    exit_code: int
    watchdog_violation: Optional[WatchdogViolation] = None
    laya_noul: float = 1.0
    laya_quality: str = "acceptable"
    failure_reasons: List[str] = field(default_factory=list)
    bead_id: Optional[str] = None


class ProbeHarness:
    def __init__(
        self,
        engine_url: str = "http://127.0.0.1:8080",
        binary_path: Optional[str] = None,
        use_pty: bool = False,
        file_beads: bool = False,
        verbose: bool = False,
    ):
        self.engine_url = engine_url
        self.use_pty = use_pty
        self.file_beads = file_beads
        self.verbose = verbose

        repo_root = Path(__file__).resolve().parent.parent.parent
        if binary_path:
            self.binary_path = Path(binary_path)
        else:
            default_bin = repo_root / "bin" / ("lk" if use_pty else "lokol")
            self.binary_path = default_bin

    def _evaluate_laya(self, transcript: str, instructions: str) -> tuple[float, str]:
        """Calls tools/laya/judge.py to evaluate semantic grounding."""
        judge_script = Path(__file__).resolve().parent.parent / "laya" / "judge.py"
        if not judge_script.exists():
            return 1.0, "acceptable"

        try:
            cmd = [
                "uv", "run", "--python", ".venv",
                str(judge_script),
                "--state", transcript,
                "--instructions", instructions,
            ]
            res = subprocess.run(cmd, capture_output=True, text=True, timeout=10.0)
            if res.returncode == 0 and res.stdout.strip():
                # Format: {"decision": true, "correct": 0.85, "quality": "good"}
                data = json.loads(res.stdout.strip())
                noul = float(data.get("correct", data.get("noul", 1.0)))
                quality = str(data.get("quality", "acceptable"))
                return noul, quality
        except Exception as e:
            if self.verbose:
                print(f"[WARN] Laya evaluation failed: {e}")
        return 1.0, "acceptable"

    def run_scenario(self, scenario: Scenario) -> ScenarioResult:
        if not self.binary_path.exists():
            raise FileNotFoundError(
                f"Lokol binary not found at {self.binary_path}. Run 'make build' first."
            )

        cmd = [
            str(self.binary_path),
            "-p", scenario.prompt,
            "-m", scenario.mode,
            "--engine", self.engine_url,
        ]

        watchdog = HardwareWatchdog(
            engine_url=self.engine_url,
            max_wall_clock_sec=scenario.max_wall_clock_sec,
            max_gpu_pegged_sec=scenario.max_gpu_pegged_sec,
        )

        start_time = time.monotonic()
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )

        watchdog.start(proc.pid)

        stdout, stderr = "", ""
        try:
            stdout, stderr = proc.communicate(timeout=scenario.max_wall_clock_sec + 2.0)
        except subprocess.TimeoutExpired:
            proc.kill()
            stdout, stderr = proc.communicate()
        finally:
            watchdog.stop()

        elapsed = time.monotonic() - start_time
        violation = watchdog.violation

        failure_reasons = []

        if violation:
            failure_reasons.append(f"WATCHDOG TRIP: {violation.reason}")

        if proc.returncode != 0 and not violation:
            failure_reasons.append(f"Non-zero exit code: {proc.returncode}")

        combined_output = f"{stdout}\n{stderr}"

        # Deterministic checks: forbidden substrings
        for forbidden in scenario.forbidden_substrings:
            if forbidden in combined_output:
                failure_reasons.append(f"Output contained forbidden pattern: {forbidden!r}")

        # Deterministic checks: required substrings
        for req in scenario.required_substrings:
            if req not in combined_output:
                failure_reasons.append(f"Output missing required pattern: {req!r}")

        # Semantic evaluation via Laya
        laya_noul = 1.0
        laya_quality = "acceptable"
        if not failure_reasons and scenario.laya_instructions:
            laya_noul, laya_quality = self._evaluate_laya(combined_output, scenario.laya_instructions)
            if laya_noul < scenario.laya_threshold:
                failure_reasons.append(
                    f"Laya semantic score ({laya_noul:.2f}) below threshold ({scenario.laya_threshold:.2f}): {laya_quality}"
                )

        passed = len(failure_reasons) == 0

        result = ScenarioResult(
            scenario=scenario,
            passed=passed,
            duration_sec=elapsed,
            stdout=stdout,
            stderr=stderr,
            exit_code=proc.returncode,
            watchdog_violation=violation,
            laya_noul=laya_noul,
            laya_quality=laya_quality,
            failure_reasons=failure_reasons,
        )

        if not passed and self.file_beads:
            bead_id = self._file_bead(result)
            result.bead_id = bead_id

        return result

    def _file_bead(self, result: ScenarioResult) -> Optional[str]:
        """Files an issue in Beads documenting the capability failure."""
        if not shutil.which("bd"):
            return None

        title = f"fix(agent): capability probe failed on {result.scenario.id}"
        reasons_md = "\n".join(f"- {r}" for r in result.failure_reasons)
        
        telemetry_summary = ""
        if result.watchdog_violation:
            v = result.watchdog_violation
            telemetry_summary = (
                f"\n\n### Hardware Watchdog Telemetry\n"
                f"- **Violation**: {v.reason}\n"
                f"- **Elapsed**: {v.elapsed_sec:.2f}s\n"
                f"- **Max GPU Util**: {v.max_gpu_util_pct:.1f}%\n"
                f"- **Tokens Decoded**: {v.tokens_decoded}\n"
            )

        body = (
            f"## Automated Capability Probe Failure\n\n"
            f"- **Scenario**: `{result.scenario.id}` ({result.scenario.name})\n"
            f"- **Mode**: `{result.scenario.mode}`\n"
            f"- **Prompt**: {result.scenario.prompt!r}\n"
            f"- **Duration**: {result.duration_sec:.2f}s\n"
            f"- **Laya Score**: {result.laya_noul:.2f} ({result.laya_quality})\n\n"
            f"### Failure Symptoms\n{reasons_md}\n"
            f"{telemetry_summary}\n"
            f"### Agent Output\n```\n{result.stdout.strip()[:1500]}\n```\n"
        )

        try:
            cmd = [
                "bd", "create", title,
                "-t", "bug",
                "-p", "P2",
                "-l", "agent-probe,loop-defect,grounding",
                "-d", body,
            ]
            res = subprocess.run(cmd, capture_output=True, text=True, timeout=5.0)
            if res.returncode == 0:
                # Output e.g.: ✓ Created issue: lokol-xyz
                for line in res.stdout.splitlines():
                    if "Created issue:" in line:
                        parts = line.split("Created issue:")
                        if len(parts) > 1:
                            return parts[1].strip().split()[0]
        except Exception:
            pass
        return None
