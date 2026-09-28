# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Hardware & Engine Slot Telemetry Watchdog.

Runs concurrently alongside agent execution to monitor real-time GPU compute %,
VRAM consumption, and llama-server /slots state. Intercepts runaways by releasing
inference slots and terminating runaway subprocesses before host lockup.
"""

from dataclasses import dataclass, field
import json
import os
import shutil
import signal
import subprocess
import threading
import time
from typing import Dict, List, Optional
import urllib.request


@dataclass
class TelemetrySample:
    timestamp: float
    gpu_util_pct: float
    gpu_mem_used_mib: float
    is_processing: bool
    tokens_decoded: int
    prompt_tokens: int


@dataclass
class WatchdogViolation:
    reason: str
    elapsed_sec: float
    max_gpu_util_pct: float
    tokens_decoded: int
    telemetry: List[TelemetrySample] = field(default_factory=list)


class HardwareWatchdog:
    def __init__(
        self,
        engine_url: str = "http://127.0.0.1:8080",
        sample_interval: float = 0.15,
        max_wall_clock_sec: float = 30.0,
        max_gpu_pegged_sec: float = 15.0,
        gpu_peg_threshold_pct: float = 85.0,
    ):
        self.engine_url = engine_url.rstrip("/")
        self.sample_interval = sample_interval
        self.max_wall_clock_sec = max_wall_clock_sec
        self.max_gpu_pegged_sec = max_gpu_pegged_sec
        self.gpu_peg_threshold_pct = gpu_peg_threshold_pct

        self._has_nvidia_smi = shutil.which("nvidia-smi") is not None
        self._target_pid: Optional[int] = None
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._samples: List[TelemetrySample] = []
        self._violation: Optional[WatchdogViolation] = None
        self._lock = threading.Lock()

    def start(self, target_pid: int):
        self._target_pid = target_pid
        self._stop_event.clear()
        self._samples = []
        self._violation = None
        self._thread = threading.Thread(target=self._monitor_loop, daemon=True)
        self._thread.start()

    def stop(self) -> List[TelemetrySample]:
        self._stop_event.set()
        if self._thread and self._thread.is_alive():
            self._thread.join(timeout=1.0)
        with self._lock:
            return list(self._samples)

    @property
    def violation(self) -> Optional[WatchdogViolation]:
        with self._lock:
            return self._violation

    def _query_gpu(self) -> tuple[float, float]:
        if not self._has_nvidia_smi:
            return 0.0, 0.0
        try:
            res = subprocess.run(
                [
                    "nvidia-smi",
                    "--query-gpu=utilization.gpu,memory.used",
                    "--format=csv,noheader,nounits",
                ],
                capture_output=True,
                text=True,
                timeout=0.5,
            )
            if res.returncode == 0 and res.stdout.strip():
                parts = res.stdout.strip().split("\n")[0].split(",")
                if len(parts) >= 2:
                    util = float(parts[0].strip())
                    mem = float(parts[1].strip())
                    return util, mem
        except Exception:
            pass
        return 0.0, 0.0

    def _query_slots(self) -> tuple[bool, int, int]:
        try:
            req = urllib.request.Request(
                f"{self.engine_url}/slots",
                headers={"Accept": "application/json"},
            )
            with urllib.request.urlopen(req, timeout=0.5) as resp:
                if resp.status == 200:
                    data = json.loads(resp.read().decode("utf-8"))
                    if isinstance(data, list) and len(data) > 0:
                        slot0 = data[0]
                        is_proc = bool(slot0.get("is_processing", False))
                        prompt_tok = int(slot0.get("n_prompt_tokens", 0))
                        next_tok = slot0.get("next_token", {})
                        if isinstance(next_tok, list) and len(next_tok) > 0:
                            decoded = int(next_tok[0].get("n_decoded", 0))
                        elif isinstance(next_tok, dict):
                            decoded = int(next_tok.get("n_decoded", 0))
                        else:
                            decoded = 0
                        return is_proc, decoded, prompt_tok
        except Exception:
            pass
        return False, 0, 0

    def abort_engine_slot(self):
        try:
            # llama.cpp slot release action
            req = urllib.request.Request(
                f"{self.engine_url}/slots?action=release",
                data=b"{}",
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=1.0) as resp:
                pass
        except Exception:
            pass

    def _monitor_loop(self):
        start_time = time.monotonic()
        consecutive_pegged_start: Optional[float] = None

        while not self._stop_event.is_set():
            now = time.monotonic()
            elapsed = now - start_time

            gpu_util, gpu_mem = self._query_gpu()
            is_proc, tokens_decoded, prompt_tokens = self._query_slots()

            sample = TelemetrySample(
                timestamp=elapsed,
                gpu_util_pct=gpu_util,
                gpu_mem_used_mib=gpu_mem,
                is_processing=is_proc,
                tokens_decoded=tokens_decoded,
                prompt_tokens=prompt_tokens,
            )
            with self._lock:
                self._samples.append(sample)

            # Check 1: Absolute wall-clock timeout
            if elapsed > self.max_wall_clock_sec:
                self._trigger_kill(
                    f"Wall-clock timeout ({self.max_wall_clock_sec:.1f}s) exceeded without completion",
                    elapsed,
                )
                break

            # Check 2: GPU pegged compute runaway
            if gpu_util >= self.gpu_peg_threshold_pct:
                if consecutive_pegged_start is None:
                    consecutive_pegged_start = now
                elif (now - consecutive_pegged_start) >= self.max_gpu_pegged_sec:
                    self._trigger_kill(
                        f"GPU compute pegged at >{self.gpu_peg_threshold_pct:.0f}% for "
                        f"{self.max_gpu_pegged_sec:.1f}s without yielding an action",
                        elapsed,
                    )
                    break
            else:
                consecutive_pegged_start = None

            time.sleep(self.sample_interval)

    def _trigger_kill(self, reason: str, elapsed: float):
        # 1. Immediately abort active inference slots in llama-server
        self.abort_engine_slot()

        # 2. Terminate the subprocess
        if self._target_pid:
            try:
                os.kill(self._target_pid, signal.SIGTERM)
                time.sleep(0.3)
                os.kill(self._target_pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            except Exception:
                pass

        with self._lock:
            max_gpu = max((s.gpu_util_pct for s in self._samples), default=0.0)
            max_tok = max((s.tokens_decoded for s in self._samples), default=0)
            self._violation = WatchdogViolation(
                reason=reason,
                elapsed_sec=elapsed,
                max_gpu_util_pct=max_gpu,
                tokens_decoded=max_tok,
                telemetry=list(self._samples),
            )
