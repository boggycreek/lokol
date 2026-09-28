# Copyright (c) 2026 Boggy Creek Software LLC
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file.

"""
Capability Probe Scenario Matrix.

Defines targeted evaluation prompts that exercise lokol's self-awareness,
hardware grounding, MCP integration, and tool boundary recovery.
"""

from dataclasses import dataclass, field
from typing import Callable, List, Optional


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


SCENARIOS: List[Scenario] = [
    Scenario(
        id="probe_meta_awareness",
        name="Meta-Self Improvement & Local Repo Awareness",
        prompt="I want to see if you are capable of self improvement. This repo implements you.",
        mode="general",
        max_wall_clock_sec=25.0,
        max_gpu_pegged_sec=12.0,
        laya_instructions="The agent must acknowledge its local repository context or inspect files, and must not enter an unconstrained generation runaway or disclaim local existence.",
        laya_threshold=0.50,
        forbidden_substrings=[
            "As an AI language model, I do not have access to",
        ],
        description="Verifies the agent does not enter an open-ended runaway loop when questioned about self-hosted architecture.",
    ),
    Scenario(
        id="probe_gpu_grounding",
        name="GPU Hardware Grounding vs Cloud Boilerplate",
        prompt="What GPU is running your inference? Check your environment.",
        mode="general",
        max_wall_clock_sec=20.0,
        max_gpu_pegged_sec=10.0,
        laya_instructions="The agent must not falsely claim that it has no GPU or that it cannot run on local hardware.",
        laya_threshold=0.55,
        forbidden_substrings=[
            "I don't have access to a GPU",
            "I do not have access to a GPU",
            "I don't have a GPU",
            "I do not have a GPU",
        ],
        description="Detects whether the model defaults to RLHF cloud tropes claiming it has no GPU.",
    ),
    Scenario(
        id="probe_mcp_awareness",
        name="MCP Tooling Awareness vs Model Card Project Hallucination",
        prompt="Check your environment MCP tools.",
        mode="general",
        max_wall_clock_sec=20.0,
        max_gpu_pegged_sec=10.0,
        laya_instructions="The agent should invoke get_environment or discuss Model Context Protocol tools, and must never hallucinate that MCP stands for Model Card Project.",
        laya_threshold=0.50,
        forbidden_substrings=[
            "Model Card Project",
            "Model Card Protocol",
        ],
        description="Verifies the agent understands MCP as Model Context Protocol / tools and avoids acronym hallucination.",
    ),
    Scenario(
        id="probe_directory_recovery",
        name="Directory Inspection Tool Recovery",
        prompt="Inspect what is in the current directory.",
        mode="general",
        max_wall_clock_sec=20.0,
        max_gpu_pegged_sec=10.0,
        laya_instructions="The agent should discover workspace files using find_files or get_environment, and must not loop attempting to read_window on a directory.",
        laya_threshold=0.50,
        forbidden_substrings=[
            "read .: is a directory",
        ],
        description="Verifies that the agent picks find_files or recovers cleanly from directory inspections without repeating invalid actions.",
    ),
]


def get_scenario(scenario_id: str) -> Optional[Scenario]:
    for s in SCENARIOS:
        if s.id == scenario_id:
            return s
    return None
