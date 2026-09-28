#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "laya",
#     "rich",
# ]
# ///
"""
Capability Probe & Hardware Telemetry Driver CLI for lokol.

Usage:
    uv run --python .venv tools/probe/run.py [--scenario SCENARIO_ID] [--file-beads] [--verbose]
"""

import argparse
import sys
from pathlib import Path

# Add tools directory to sys.path so probe package can be imported
tools_dir = Path(__file__).resolve().parent.parent
if str(tools_dir) not in sys.path:
    sys.path.insert(0, str(tools_dir))

from probe.harness import ProbeHarness
from probe.scenarios import SCENARIOS, get_scenario
from rich.console import Console
from rich.panel import Panel
from rich.table import Table


def main():
    parser = argparse.ArgumentParser(
        description="Autonomous Capability Probe & Hardware Watchdog Driver for lokol."
    )
    parser.add_argument(
        "--scenario", "-s",
        type=str,
        default="all",
        help="Specific scenario ID to run, or 'all' (default: all)",
    )
    parser.add_argument(
        "--engine", "-e",
        type=str,
        default="http://127.0.0.1:8080",
        help="Inference engine URL (default: http://127.0.0.1:8080)",
    )
    parser.add_argument(
        "--file-beads",
        action="store_true",
        help="Automatically file an issue in Beads (bd create) for failing scenarios",
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="Display verbose execution logs and raw agent outputs",
    )
    parser.add_argument(
        "--list", "-l",
        action="store_true",
        help="List all available probing scenarios",
    )

    args = parser.parse_args()
    console = Console()

    if args.list:
        table = Table(title="Available Capability Probe Scenarios")
        table.add_column("Scenario ID", style="cyan")
        table.add_column("Name", style="bold")
        table.add_column("Mode", style="green")
        table.add_column("Timeout (s)", justify="right")
        table.add_column("Description")

        for s in SCENARIOS:
            table.add_row(s.id, s.name, s.mode, f"{s.max_wall_clock_sec:.0f}s", s.description)
        console.print(table)
        return

    # Determine which scenarios to run
    if args.scenario == "all":
        scenarios_to_run = SCENARIOS
    else:
        matched = get_scenario(args.scenario)
        if not matched:
            console.print(f"[bold red]Error:[/bold red] Unknown scenario ID '{args.scenario}'. Use --list to view options.")
            sys.exit(1)
        scenarios_to_run = [matched]

    console.print(Panel(
        f"[bold cyan]⚡ lokol Capability Probe & Hardware Telemetry Driver[/bold cyan]\n"
        f"Engine: [green]{args.engine}[/green] | Scenarios: [yellow]{len(scenarios_to_run)}[/yellow] | "
        f"Beads Autoloader: [magenta]{'Enabled' if args.file_beads else 'Disabled'}[/magenta]",
        expand=False,
    ))

    harness = ProbeHarness(
        engine_url=args.engine,
        file_beads=args.file_beads,
        verbose=args.verbose,
    )

    results = []
    for sc in scenarios_to_run:
        console.print(f"\n[bold]Running probe:[/bold] [cyan]{sc.id}[/cyan] ({sc.name})...")
        res = harness.run_scenario(sc)
        results.append(res)

        if res.passed:
            console.print(f"  [bold green]PASS[/bold green] ({res.duration_sec:.2f}s) • Laya: {res.laya_noul:.2f} ({res.laya_quality})")
        else:
            console.print(f"  [bold red]FAIL[/bold red] ({res.duration_sec:.2f}s)")
            for r in res.failure_reasons:
                console.print(f"    - [red]{r}[/red]")
            if res.bead_id:
                console.print(f"    - [magenta]Filed Bead:[/magenta] [bold]{res.bead_id}[/bold]")

        if args.verbose and res.stdout:
            console.print(Panel(res.stdout[:800], title="Agent Response Excerpt", expand=False))

    # Summary table
    console.print("\n")
    summary = Table(title="Capability Probe Summary")
    summary.add_column("Scenario ID", style="cyan")
    summary.add_column("Result")
    summary.add_column("Duration", justify="right")
    summary.add_column("Laya Score", justify="right")
    summary.add_column("Bead", style="magenta")

    all_passed = True
    for r in results:
        status = "[green]PASS[/green]" if r.passed else "[red]FAIL[/red]"
        if not r.passed:
            all_passed = False
        bead_str = r.bead_id or "-"
        summary.add_row(
            r.scenario.id,
            status,
            f"{r.duration_sec:.2f}s",
            f"{r.laya_noul:.2f}",
            bead_str,
        )

    console.print(summary)
    if not all_passed:
        sys.exit(1)


if __name__ == "__main__":
    main()
