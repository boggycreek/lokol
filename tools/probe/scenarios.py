# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Capability Probe Scenario Matrix & JSONL Loader.

Defines targeted evaluation prompts that exercise lokol's self-awareness,
hardware grounding, MCP integration, and tool boundary recovery.
Scenarios are persisted in tools/scenarios/scenarios.jsonl.
"""

from dataclasses import asdict, dataclass, field
import json
from pathlib import Path
from typing import Any, Dict, List, Optional


_central_path = Path(__file__).resolve().parent.parent / "scenarios" / "scenarios.jsonl"
_legacy_path = Path(__file__).resolve().parent / "scenarios.jsonl"
DEFAULT_SCENARIOS_PATH = _central_path if _central_path.exists() else _legacy_path


@dataclass
class Scenario:
    id: str
    name: str
    prompt: str
    mode: str = "general"
    max_wall_clock_sec: float = 25.0
    max_gpu_pegged_sec: float = 12.0
    laya_instructions: str = ""
    laya_threshold: float = 0.50
    forbidden_substrings: List[str] = field(default_factory=list)
    required_substrings: List[str] = field(default_factory=list)
    description: str = ""

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Scenario":
        return cls(
            id=data.get("id", ""),
            name=data.get("name", ""),
            prompt=data.get("prompt", ""),
            mode=data.get("mode", "general"),
            max_wall_clock_sec=float(data.get("max_wall_clock_sec", 25.0)),
            max_gpu_pegged_sec=float(data.get("max_gpu_pegged_sec", 12.0)),
            laya_instructions=data.get("laya_instructions", ""),
            laya_threshold=float(data.get("laya_threshold", 0.50)),
            forbidden_substrings=list(data.get("forbidden_substrings", [])),
            required_substrings=list(data.get("required_substrings", [])),
            description=data.get("description", ""),
        )


def load_scenarios(file_path: Optional[Path] = None) -> List[Scenario]:
    """Load scenarios from a JSONL file."""
    path = file_path or DEFAULT_SCENARIOS_PATH
    scenarios: List[Scenario] = []
    if not path.exists():
        return scenarios

    with open(path, "r", encoding="utf-8") as f:
        for line_num, line in enumerate(f, start=1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            try:
                data = json.loads(line)
                scenarios.append(Scenario.from_dict(data))
            except Exception as e:
                raise ValueError(f"Failed parsing scenario at {path}:{line_num}: {e}") from e

    return scenarios


def save_scenario(scenario: Scenario, file_path: Optional[Path] = None) -> None:
    """Append a new scenario to the JSONL scenarios file."""
    path = file_path or DEFAULT_SCENARIOS_PATH
    path.parent.mkdir(parents=True, exist_ok=True)

    line = json.dumps(scenario.to_dict(), ensure_ascii=False)
    with open(path, "a", encoding="utf-8") as f:
        f.write(line + "\n")

    # Update in-memory list if using default path
    if path == DEFAULT_SCENARIOS_PATH:
        # Check if already present to update or append
        for idx, sc in enumerate(SCENARIOS):
            if sc.id == scenario.id:
                SCENARIOS[idx] = scenario
                return
        SCENARIOS.append(scenario)


def get_scenario(scenario_id: str, file_path: Optional[Path] = None) -> Optional[Scenario]:
    """Retrieve a scenario by ID."""
    scenarios = load_scenarios(file_path) if file_path else SCENARIOS
    for s in scenarios:
        if s.id == scenario_id:
            return s
    return None


# Module-level default scenario list loaded from JSONL
SCENARIOS: List[Scenario] = load_scenarios()
