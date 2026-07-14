#!/usr/bin/env python3
"""Audit the global Summer Harness installation without modifying it."""

from __future__ import annotations

import json
import pathlib
import re
import subprocess
import sys
import tempfile


HOME = pathlib.Path.home()
REPO = pathlib.Path(__file__).resolve().parents[1]
CODEX = HOME / ".codex"


def resolved(path: pathlib.Path) -> pathlib.Path:
    try:
        return path.resolve(strict=True)
    except FileNotFoundError:
        return pathlib.Path("/__missing__")


def main() -> int:
    checks = []

    def check(name: str, ok: bool, detail: str) -> None:
        checks.append({"name": name, "ok": bool(ok), "detail": detail})

    expected_links = {
        CODEX / "AGENTS.md": REPO / "config" / "AGENTS.md",
        CODEX / "skills" / "summer-harness": REPO / "skills" / "summer-harness",
        CODEX / "skills" / "project-handoff": REPO / "skills" / "project-handoff",
        CODEX / "skills" / "adaptive-harness-router": REPO / "skills" / "adaptive-harness-router",
    }
    for actual, expected in expected_links.items():
        check(f"link:{actual.name}", resolved(actual) == expected.resolve(), f"{actual} -> {resolved(actual)}")

    agents = (REPO / "config" / "AGENTS.md").read_bytes()
    check("agents-size", len(agents) <= 5000, f"{len(agents)} bytes")

    hooks_path = CODEX / "hooks.json"
    try:
        hooks = json.loads(hooks_path.read_text(encoding="utf-8"))
        raw_hooks = json.dumps(hooks)
        check("hooks-json", True, "valid JSON")
        check("legacy-hooks", "harness-" not in raw_hooks and "gsd-check-update" not in raw_hooks,
              "no implicit Harness/GSD session hooks")
        personal_path = re.search(r"/(?:Users|home)/[^/$\"']+|[A-Za-z]:\\\\Users\\\\[^\\\"']+", raw_hooks)
        check("hooks-portable-paths", personal_path is None,
              personal_path.group(0) if personal_path else "no developer-specific home path")
    except (OSError, json.JSONDecodeError) as exc:
        check("hooks-json", False, str(exc))

    public_hooks_path = REPO / "config" / "hooks.json"
    try:
        public_hooks = json.loads(public_hooks_path.read_text(encoding="utf-8"))
        check("public-hooks-safe", public_hooks == {"hooks": {}}, "empty opt-in example")
    except (OSError, json.JSONDecodeError) as exc:
        check("public-hooks-safe", False, str(exc))

    for name in ("grilling", "diagnosing-bugs", "codebase-design", "domain-modeling", "tdd"):
        path = CODEX / "skills" / name / "SKILL.md"
        check(f"matt:{name}", path.is_file(), str(path))

    router_files = sorted(
        str(path.relative_to(REPO / "skills" / "adaptive-harness-router"))
        for path in (REPO / "skills" / "adaptive-harness-router").rglob("*")
        if path.is_file()
    )
    check("router-surface", router_files == ["SKILL.md", "agents/openai.yaml"], ", ".join(router_files))
    check("ask-matt-disabled", not (CODEX / "skills" / "ask-matt").exists(),
          "no second lifecycle router in the active skill surface")

    cli = REPO / "skills" / "summer-harness" / "scripts" / "harnessctl.py"
    with tempfile.TemporaryDirectory() as temp:
        command = [sys.executable, str(cli), "--repo", temp, "handoff", "--mode", "direct",
                   "--goal", "doctor probe", "--next", "resume"]
        result = subprocess.run(command, text=True, capture_output=True)
        files = sorted(str(path.relative_to(temp)) for path in pathlib.Path(temp).rglob("*") if path.is_file())
        directories = sorted(str(path.relative_to(temp)) for path in pathlib.Path(temp).rglob("*") if path.is_dir())
        check("direct-handoff-only", result.returncode == 0 and files == [".agent/HANDOFF.md"] and directories == [".agent"],
              f"exit={result.returncode}, files={files}, dirs={directories}")

    surface_path = CODEX / ".gsd-surface.json"
    try:
        surface = json.loads(surface_path.read_text(encoding="utf-8"))
        check("gsd-profile", surface.get("baseProfile") == "standard", json.dumps(surface, ensure_ascii=False))
    except (OSError, json.JSONDecodeError) as exc:
        check("gsd-profile", False, str(exc))
    gsd_count = len(list((CODEX / "skills").glob("gsd-*")))
    check("gsd-surface-size", 10 <= gsd_count <= 25, f"{gsd_count} active GSD skills")
    gsd_source = CODEX / ".gsd-source"
    source_path = pathlib.Path(gsd_source.read_text(encoding="utf-8").strip()) if gsd_source.exists() else pathlib.Path("/__missing__")
    check("gsd-source", source_path.is_dir(), str(source_path))
    check("gsd-installer-runtime", (CODEX / "bin" / "install.js").is_file(), str(CODEX / "bin" / "install.js"))
    gsd_surface_text = (CODEX / "skills" / "gsd-surface" / "SKILL.md").read_text(encoding="utf-8")
    check("gsd-codex-path", "${CODEX_HOME:-$HOME/.codex}" in gsd_surface_text and "CLAUDE_CONFIG_DIR" not in gsd_surface_text,
          "surface runtime points to CODEX_HOME")

    gstack_count = len([path for path in (CODEX / "skills").glob("gstack-*") if path.is_symlink()])
    check("gstack-surface-size", gstack_count == 8, f"{gstack_count} active gstack skills")

    conflicts = [
        CODEX / "skills" / "coding-agent-harness",
        CODEX / "skills" / "harness",
        HOME / ".agents" / "skills" / "super-dev",
        HOME / ".agents" / "skills" / "super-dev-seeai",
    ]
    active_conflicts = [str(path) for path in conflicts if path.exists() or path.is_symlink()]
    check("legacy-surfaces", not active_conflicts, ", ".join(active_conflicts) or "none active")

    ok = all(item["ok"] for item in checks)
    print(json.dumps({"ok": ok, "checks": checks}, ensure_ascii=False, indent=2))
    return 0 if ok else 2


if __name__ == "__main__":
    raise SystemExit(main())
