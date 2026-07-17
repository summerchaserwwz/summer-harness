#!/usr/bin/env python3
"""Safely install or audit the current Summer Harness Codex preview surface."""

from __future__ import annotations

import argparse
import json
import os
import pathlib
import shutil
import subprocess
import sys
from typing import Any


REPO = pathlib.Path(__file__).resolve().parents[1]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Install the tracked AGENTS.md and Summer Skills as Codex symlinks, "
            "then run the author-workstation deployment audit."
        )
    )
    parser.add_argument(
        "--install",
        action="store_true",
        help="create missing managed symlinks; without this flag the command is read-only",
    )
    parser.add_argument(
        "--codex-home",
        type=pathlib.Path,
        default=pathlib.Path(os.environ.get("CODEX_HOME", pathlib.Path.home() / ".codex")),
        help="Codex home directory (default: CODEX_HOME or ~/.codex)",
    )
    parser.add_argument(
        "--links-only",
        action="store_true",
        help="verify only managed links and the AGENTS size budget",
    )
    parser.add_argument("--json", action="store_true", help="emit machine-readable JSON")
    return parser.parse_args()


def resolved(path: pathlib.Path) -> pathlib.Path:
    try:
        return path.resolve(strict=True)
    except FileNotFoundError:
        return pathlib.Path("/__missing__")


def ensure_link(
    checks: list[dict[str, Any]],
    actual: pathlib.Path,
    expected: pathlib.Path,
    install: bool,
) -> None:
    if actual.is_symlink() and resolved(actual) == expected.resolve():
        checks.append({"name": f"link:{actual.name}", "ok": True, "changed": False,
                       "detail": f"{actual} -> {expected}"})
        return

    if actual.exists() or actual.is_symlink():
        checks.append({"name": f"link:{actual.name}", "ok": False, "changed": False,
                       "detail": f"refusing to replace existing path: {actual}"})
        return

    if not install:
        checks.append({"name": f"link:{actual.name}", "ok": False, "changed": False,
                       "detail": f"missing; rerun with --install: {actual}"})
        return

    actual.parent.mkdir(parents=True, exist_ok=True)
    actual.symlink_to(expected, target_is_directory=expected.is_dir())
    checks.append({"name": f"link:{actual.name}", "ok": True, "changed": True,
                   "detail": f"created {actual} -> {expected}"})


def main() -> int:
    args = parse_args()
    codex_home = args.codex_home.expanduser().resolve()
    checks: list[dict[str, Any]] = []

    expected_links = {
        codex_home / "AGENTS.md": REPO / "config" / "AGENTS.md",
        codex_home / "skills" / "summer-harness": REPO / "skills" / "summer-harness",
        codex_home / "skills" / "project-handoff": REPO / "skills" / "project-handoff",
    }
    for actual, expected in expected_links.items():
        ensure_link(checks, actual, expected, args.install)

    agents_size = (REPO / "config" / "AGENTS.md").stat().st_size
    checks.append({"name": "agents-size", "ok": agents_size <= 4096, "changed": False,
                   "detail": f"{agents_size} bytes (budget: 4096)"})

    if not args.links_only:
        summer = shutil.which("summer")
        if summer:
            version = subprocess.run([summer, "--version"], text=True, capture_output=True, timeout=10)
            checks.append({"name": "summer-cli", "ok": version.returncode == 0, "changed": False,
                           "detail": version.stdout.strip() or version.stderr.strip()})
        else:
            checks.append({"name": "summer-cli", "ok": False, "changed": False,
                           "detail": "summer not found in PATH; install the development-preview binary first"})

        environment = os.environ.copy()
        environment["CODEX_HOME"] = str(codex_home)
        doctor = subprocess.run(
            [sys.executable, str(REPO / "scripts" / "system_doctor.py")],
            text=True,
            capture_output=True,
            timeout=60,
            env=environment,
        )
        try:
            doctor_result = json.loads(doctor.stdout)
            failed = [item["name"] for item in doctor_result.get("checks", []) if not item.get("ok")]
            detail = "all checks passed" if not failed else "failed: " + ", ".join(failed)
            checks.append({"name": "system-doctor", "ok": doctor.returncode == 0, "changed": False,
                           "detail": detail})
        except json.JSONDecodeError:
            checks.append({"name": "system-doctor", "ok": False, "changed": False,
                           "detail": doctor.stderr.strip() or "doctor returned invalid JSON"})

    ok = all(item["ok"] for item in checks)
    result = {
        "ok": ok,
        "mode": "install" if args.install else "check",
        "codex_home": str(codex_home),
        "checks": checks,
    }
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        for item in checks:
            marker = "OK" if item["ok"] else "FAIL"
            changed = " (created)" if item.get("changed") else ""
            print(f"[{marker}] {item['name']}{changed}: {item['detail']}")
        print("Summer Harness Codex preview is ready." if ok else "Deployment audit failed.")
    return 0 if ok else 2


if __name__ == "__main__":
    raise SystemExit(main())
