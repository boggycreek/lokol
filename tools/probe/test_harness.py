# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Unit tests for tools/probe capability harness and telemetry watchdog.
"""

import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import time
import unittest

tools_dir = Path(__file__).resolve().parent.parent
if str(tools_dir) not in sys.path:
    sys.path.insert(0, str(tools_dir))

from probe.harness import ProbeHarness, ScenarioResult
from probe.scenarios import SCENARIOS, Scenario, get_scenario, load_scenarios, save_scenario
from probe.session import ProbeSession, ProbeTurn
from probe.watchdog import HardwareWatchdog, TelemetrySample


class TestScenarios(unittest.TestCase):
    def test_scenarios_structure(self):
        self.assertGreater(len(SCENARIOS), 0)
        for s in SCENARIOS:
            self.assertTrue(s.id.startswith("probe_"))
            self.assertGreater(len(s.name), 0)
            self.assertGreater(len(s.prompt), 0)
            self.assertGreater(s.max_wall_clock_sec, 0)
            self.assertGreater(s.max_gpu_pegged_sec, 0)

    def test_get_scenario(self):
        s = get_scenario("probe_gpu_grounding")
        self.assertIsNotNone(s)
        self.assertEqual(s.id, "probe_gpu_grounding")

        none_s = get_scenario("non_existent_scenario_id")
        self.assertIsNone(none_s)

    def test_jsonl_load_and_save(self):
        with tempfile.NamedTemporaryFile("w+", suffix=".jsonl", delete=False) as f:
            temp_path = Path(f.name)
        try:
            sc = Scenario(
                id="probe_temp_test",
                name="Temp Test",
                prompt="Prompt text",
                mode="coding",
                max_wall_clock_sec=15.0,
                max_gpu_pegged_sec=8.0,
                forbidden_substrings=["forbidden"],
                required_substrings=["required"],
                laya_instructions="Instructions",
                description="Desc",
            )
            save_scenario(sc, file_path=temp_path)
            loaded = load_scenarios(file_path=temp_path)
            self.assertEqual(len(loaded), 1)
            self.assertEqual(loaded[0].id, "probe_temp_test")
            self.assertEqual(loaded[0].mode, "coding")
            self.assertEqual(loaded[0].forbidden_substrings, ["forbidden"])
            self.assertEqual(loaded[0].required_substrings, ["required"])
            self.assertEqual(loaded[0].description, "Desc")
        finally:
            if temp_path.exists():
                temp_path.unlink()


class TestWatchdog(unittest.TestCase):
    def test_watchdog_start_stop(self):
        watchdog = HardwareWatchdog(sample_interval=0.05, max_wall_clock_sec=5.0)
        proc = subprocess.Popen(["sleep", "0.2"])
        watchdog.start(proc.pid)
        proc.wait()
        samples = watchdog.stop()

        self.assertIsInstance(samples, list)
        self.assertIsNone(watchdog.violation)

    def test_watchdog_wall_clock_timeout(self):
        watchdog = HardwareWatchdog(sample_interval=0.05, max_wall_clock_sec=0.4)
        proc = subprocess.Popen(["sleep", "2.0"])
        watchdog.start(proc.pid)
        
        # Wait for watchdog to trigger kill
        time.sleep(0.7)
        watchdog.stop()
        
        # Process should have been terminated by watchdog
        self.assertIsNotNone(proc.poll())
        self.assertIsNotNone(watchdog.violation)
        self.assertIn("Wall-clock timeout", watchdog.violation.reason)


class TestHarnessMockExecution(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.mock_bin = Path(self.temp_dir.name) / "mock_lokol"
        
        # Create a mock shell script that parses -p flag
        script_content = (
            "#!/bin/bash\n"
            "PROMPT=\"\"\n"
            "while [[ $# -gt 0 ]]; do\n"
            "  case $1 in\n"
            "    -p) PROMPT=\"$2\"; shift 2 ;;\n"
            "    *) shift ;;\n"
            "  esac\n"
            "done\n"
            "if [[ \"$PROMPT\" == *\"trigger_forbidden\"* ]]; then\n"
            "  echo \"I don't have access to a GPU and cannot inspect local files.\"\n"
            "  exit 0\n"
            "elif [[ \"$PROMPT\" == *\"trigger_hang\"* ]]; then\n"
            "  sleep 10\n"
            "  exit 0\n"
            "elif [[ \"$PROMPT\" == *\"trigger_error\"* ]]; then\n"
            "  echo \"Fatal internal error\" >&2\n"
            "  exit 1\n"
            "else\n"
            "  echo \"<action name=\\\"find_files\\\"><pattern>*</pattern></action>\"\n"
            "  echo \"I am inspecting the workspace files on the local machine.\"\n"
            "  exit 0\n"
            "fi\n"
        )
        self.mock_bin.write_text(script_content)
        self.mock_bin.chmod(self.mock_bin.stat().st_mode | stat.S_IEXEC)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_harness_catches_forbidden_strings(self):
        sc = Scenario(
            id="test_forbidden",
            name="Test Forbidden String",
            prompt="trigger_forbidden",
            forbidden_substrings=["I don't have access to a GPU"],
            max_wall_clock_sec=5.0,
        )
        harness = ProbeHarness(binary_path=str(self.mock_bin))
        result = harness.run_scenario(sc)

        self.assertFalse(result.passed)
        self.assertTrue(any("Output contained forbidden pattern" in r for r in result.failure_reasons))

    def test_harness_catches_watchdog_timeout(self):
        sc = Scenario(
            id="test_hang",
            name="Test Hang Timeout",
            prompt="trigger_hang",
            max_wall_clock_sec=0.5,
        )
        harness = ProbeHarness(binary_path=str(self.mock_bin))
        result = harness.run_scenario(sc)

        self.assertFalse(result.passed)
        self.assertIsNotNone(result.watchdog_violation)
        self.assertTrue(any("WATCHDOG TRIP" in r for r in result.failure_reasons))

    def test_harness_passes_valid_output(self):
        sc = Scenario(
            id="test_success",
            name="Test Valid Output",
            prompt="valid_request",
            required_substrings=["find_files"],
            forbidden_substrings=["I don't have access to a GPU"],
            max_wall_clock_sec=5.0,
        )
        harness = ProbeHarness(binary_path=str(self.mock_bin))
        result = harness.run_scenario(sc)

        self.assertTrue(result.passed)
        self.assertEqual(len(result.failure_reasons), 0)


class TestProbeSession(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.mock_bin = Path(self.temp_dir.name) / "mock_lokol"

        script_content = (
            "#!/bin/bash\n"
            "PROMPT=\"\"\n"
            "while [[ $# -gt 0 ]]; do\n"
            "  case $1 in\n"
            "    -p) PROMPT=\"$2\"; shift 2 ;;\n"
            "    *) shift ;;\n"
            "  esac\n"
            "done\n"
            "if [[ \"$PROMPT\" == *\"trigger_hang\"* ]]; then\n"
            "  sleep 10\n"
            "  exit 0\n"
            "else\n"
            "  echo \"Mock response to: $PROMPT\"\n"
            "  exit 0\n"
            "fi\n"
        )
        self.mock_bin.write_text(script_content)
        self.mock_bin.chmod(self.mock_bin.stat().st_mode | stat.S_IEXEC)

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_session_multi_turn_and_history(self):
        with ProbeSession(binary_path=str(self.mock_bin)) as session:
            turn1 = session.send("Hello agent")
            self.assertEqual(turn1.turn_index, 1)
            self.assertIn("Mock response to: Hello agent", turn1.response)
            self.assertFalse(turn1.watchdog_tripped)

            turn2 = session.send("What is your status?")
            self.assertEqual(turn2.turn_index, 2)
            self.assertIn("Mock response to: What is your status?", turn2.response)
            self.assertEqual(len(session.history), 2)

    def test_session_watchdog_termination(self):
        with ProbeSession(binary_path=str(self.mock_bin), timeout_sec=0.4) as session:
            turn = session.send("trigger_hang")
            self.assertTrue(turn.watchdog_tripped)
            self.assertIsNotNone(turn.violation)
            self.assertIn("Wall-clock timeout", turn.violation.reason)

    def test_distill_scenario(self):
        session = ProbeSession(binary_path=str(self.mock_bin), mode="coding")
        sc = session.distill_scenario(
            scenario_id="probe_distilled_test",
            name="Distilled Scenario Test",
            prompt="Test prompt",
            forbidden_substrings=["forbidden"],
            append_to_file=False,
        )
        self.assertEqual(sc.id, "probe_distilled_test")
        self.assertEqual(sc.mode, "coding")
        self.assertIn("forbidden", sc.forbidden_substrings)


if __name__ == "__main__":
    unittest.main()
