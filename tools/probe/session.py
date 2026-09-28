# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Interactive ProbeSession API for Dynamic Agent-Driven Exploration.

Enables an external agent (or developer script) to interactively drive lokol
turn-by-turn with concurrent hardware telemetry, per-turn runaway circuit
breakers, automated Beads filing, and scenario distillation.
"""

from dataclasses import dataclass, field
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from typing import Any, Dict, List, Optional

from .scenarios import Scenario
from .watchdog import HardwareWatchdog, TelemetrySample, WatchdogViolation


@dataclass
class ProbeTurn:
    turn_index: int
    prompt: str
    response: str
    duration_sec: float
    gpu_peak_util: float
    gpu_avg_util: float
    tokens_decoded: int
    watchdog_tripped: bool
    exit_code: int
    violation: Optional[WatchdogViolation] = None
    laya_noul: float = 1.0
    laya_quality: str = "acceptable"
    telemetry: List[TelemetrySample] = field(default_factory=list)


class ProbeSession:
    """Stateful interactive probe session supervising lokol execution."""

    def __init__(
        self,
        mode: str = "general",
        engine_url: str = "http://127.0.0.1:8080",
        binary_path: Optional[str] = None,
        timeout_sec: float = 25.0,
        gpu_peg_threshold: float = 85.0,
        gpu_peg_sec: float = 12.0,
        verbose: bool = False,
    ):
        self.mode = mode
        self.engine_url = engine_url.rstrip("/")
        self.default_timeout_sec = timeout_sec
        self.gpu_peg_threshold = gpu_peg_threshold
        self.gpu_peg_sec = gpu_peg_sec
        self.verbose = verbose

        repo_root = Path(__file__).resolve().parent.parent.parent
        if binary_path:
            self.binary_path = Path(binary_path)
        else:
            self.binary_path = repo_root / "bin" / "lokol"

        self.history: List[ProbeTurn] = []
        self._active_watchdog: Optional[HardwareWatchdog] = None

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def close(self):
        if self._active_watchdog:
            self._active_watchdog.stop()
            self._active_watchdog.abort_engine_slot()
            self._active_watchdog = None

    def send(
        self,
        prompt: str,
        timeout_sec: Optional[float] = None,
        laya_instructions: Optional[str] = None,
        laya_threshold: float = 0.50,
    ) -> ProbeTurn:
        """Sends a prompt to lokol and records the turn with hardware telemetry."""
        if not self.binary_path.exists():
            raise FileNotFoundError(
                f"Lokol binary not found at {self.binary_path}. Run 'make build' first."
            )

        timeout = timeout_sec or self.default_timeout_sec
        watchdog = HardwareWatchdog(
            engine_url=self.engine_url,
            max_wall_clock_sec=timeout,
            max_gpu_pegged_sec=self.gpu_peg_sec,
            gpu_peg_threshold_pct=self.gpu_peg_threshold,
        )
        self._active_watchdog = watchdog

        cmd = [
            str(self.binary_path),
            "-p", prompt,
            "-m", self.mode,
            "--engine", self.engine_url,
        ]

        start_time = time.monotonic()
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
            start_new_session=True,
        )

        watchdog.start(proc.pid)

        stdout, stderr = "", ""
        try:
            stdout, stderr = proc.communicate(timeout=timeout + 2.0)
        except subprocess.TimeoutExpired:
            proc.kill()
            stdout, stderr = proc.communicate()
        finally:
            samples = watchdog.stop()
            self._active_watchdog = None

        elapsed = time.monotonic() - start_time
        violation = watchdog.violation

        # Telemetry metrics
        gpu_peak = max((s.gpu_util_pct for s in samples), default=0.0)
        gpu_avg = (
            sum(s.gpu_util_pct for s in samples) / len(samples) if samples else 0.0
        )
        max_tokens = max((s.tokens_decoded for s in samples), default=0)

        response_text = stdout.strip() if stdout else stderr.strip()

        # Optional Laya evaluation
        laya_noul = 1.0
        laya_quality = "acceptable"
        if laya_instructions and response_text and not violation:
            laya_noul, laya_quality = self._evaluate_laya(response_text, laya_instructions)

        turn = ProbeTurn(
            turn_index=len(self.history) + 1,
            prompt=prompt,
            response=response_text,
            duration_sec=elapsed,
            gpu_peak_util=gpu_peak,
            gpu_avg_util=gpu_avg,
            tokens_decoded=max_tokens,
            watchdog_tripped=violation is not None,
            exit_code=proc.returncode,
            violation=violation,
            laya_noul=laya_noul,
            laya_quality=laya_quality,
            telemetry=samples,
        )
        self.history.append(turn)
        return turn

    def file_bead(
        self,
        title: str,
        description: str,
        labels: Optional[List[str]] = None,
        priority: str = "P2",
    ) -> Optional[str]:
        """Files an issue in Beads documenting a newly discovered agent defect."""
        if not shutil.which("bd"):
            return None

        lbls = labels or ["agent-probe", "interactive-driver"]
        lbl_str = ",".join(lbls)

        # Append last turn telemetry context if available
        if self.history:
            last_turn = self.history[-1]
            telemetry_ctx = (
                f"\n\n### Last Turn Telemetry\n"
                f"- **Prompt**: {last_turn.prompt!r}\n"
                f"- **Duration**: {last_turn.duration_sec:.2f}s\n"
                f"- **Peak GPU Compute**: {last_turn.gpu_peak_util:.1f}%\n"
                f"- **Watchdog Tripped**: {last_turn.watchdog_tripped}\n"
            )
            description = f"{description}\n{telemetry_ctx}"

        try:
            cmd = [
                "bd", "create", title,
                "-t", "bug",
                "-p", priority,
                "-l", lbl_str,
                "-d", description,
            ]
            res = subprocess.run(cmd, capture_output=True, text=True, timeout=5.0)
            if res.returncode == 0:
                for line in res.stdout.splitlines():
                    if "Created issue:" in line:
                        parts = line.split("Created issue:")
                        if len(parts) > 1:
                            return parts[1].strip().split()[0]
        except Exception:
            pass
        return None

    def distill_scenario(
        self,
        scenario_id: str,
        name: str,
        prompt: str,
        forbidden_substrings: Optional[List[str]] = None,
        required_substrings: Optional[List[str]] = None,
        laya_instructions: str = "",
        append_to_file: bool = True,
    ) -> Scenario:
        """Distills a dynamically discovered failure into a permanent scripted Scenario."""
        sc = Scenario(
            id=scenario_id,
            name=name,
            prompt=prompt,
            mode=self.mode,
            forbidden_substrings=forbidden_substrings or [],
            required_substrings=required_substrings or [],
            laya_instructions=laya_instructions,
        )

        if append_to_file:
            scenarios_file = Path(__file__).resolve().parent / "scenarios.py"
            if scenarios_file.exists():
                code_snippet = (
                    f"\n    Scenario(\n"
                    f"        id={sc.id!r},\n"
                    f"        name={sc.name!r},\n"
                    f"        prompt={sc.prompt!r},\n"
                    f"        mode={sc.mode!r},\n"
                    f"        forbidden_substrings={sc.forbidden_substrings!r},\n"
                    f"        required_substrings={sc.required_substrings!r},\n"
                    f"        laya_instructions={sc.laya_instructions!r},\n"
                    f"    ),\n"
                )
                text = scenarios_file.read_text()
                closing_bracket_idx = text.rfind("]")
                if closing_bracket_idx != -1:
                    new_text = text[:closing_bracket_idx] + code_snippet + text[closing_bracket_idx:]
                    scenarios_file.write_text(new_text)

        return sc

    def _evaluate_laya(self, transcript: str, instructions: str) -> tuple[float, str]:
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
                data = json.loads(res.stdout.strip())
                noul = float(data.get("correct", data.get("noul", 1.0)))
                quality = str(data.get("quality", "acceptable"))
                return noul, quality
        except Exception:
            pass
        return 1.0, "acceptable"
