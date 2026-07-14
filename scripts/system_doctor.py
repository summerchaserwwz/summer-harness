#!/usr/bin/env python3
"""Maintainer-only audit of Summer's author workstation integration."""

from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile


HOME = pathlib.Path(os.environ.get("HOME", pathlib.Path.home())).expanduser()
REPO = pathlib.Path(__file__).resolve().parents[1]
CODEX = pathlib.Path(os.environ.get("CODEX_HOME", HOME / ".codex")).expanduser()
GSTACK = pathlib.Path(os.environ.get("GSTACK_ROOT", HOME / ".gstack" / "repos" / "gstack")).expanduser()


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
    }
    for actual, expected in expected_links.items():
        check(f"link:{actual.name}", resolved(actual) == expected.resolve(), f"{actual} -> {resolved(actual)}")

    agents = (REPO / "config" / "AGENTS.md").read_bytes()
    check("agents-size", len(agents) <= 4096, f"{len(agents)} bytes")

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
    router_link = CODEX / "skills" / "adaptive-harness-router"
    check("router-not-installed", not router_link.exists() and not router_link.is_symlink(), str(router_link))
    check("ask-matt-disabled", not (CODEX / "skills" / "ask-matt").exists(),
          "no second lifecycle router in the active skill surface")

    summer_metadata = REPO / "skills" / "summer-harness" / "agents" / "openai.yaml"
    try:
        raw = summer_metadata.read_text(encoding="utf-8")
        matches = re.findall(r"^\s*allow_implicit_invocation:\s*(true|false)\s*$", raw, re.MULTILINE)
        check("summer-explicit-only", matches == ["false"], f"allow_implicit_invocation={matches}")
    except OSError as exc:
        check("summer-explicit-only", False, str(exc))

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
        clean_surface = (surface.get("baseProfile") == "core" and
                         surface.get("disabledClusters") == [] and
                         surface.get("explicitAdds") == [] and
                         surface.get("explicitRemoves") == [])
        check("gsd-profile", clean_surface, json.dumps(surface, ensure_ascii=False))
    except (OSError, json.JSONDecodeError) as exc:
        check("gsd-profile", False, str(exc))
    expected_gsd = {
        "gsd-new-project", "gsd-discuss-phase", "gsd-plan-phase", "gsd-execute-phase",
        "gsd-phase", "gsd-help", "gsd-update", "gsd-surface",
    }
    active_gsd = {path.name for path in (CODEX / "skills").glob("gsd-*") if path.is_dir()}
    check("gsd-surface", active_gsd == expected_gsd,
          f"active={sorted(active_gsd)}, expected={sorted(expected_gsd)}")
    gsd_source = CODEX / ".gsd-source"
    source_path = pathlib.Path(gsd_source.read_text(encoding="utf-8").strip()) if gsd_source.exists() else pathlib.Path("/__missing__")
    check("gsd-source", source_path.is_dir(), str(source_path))
    gsd_surface_skill = CODEX / "skills" / "gsd-surface" / "SKILL.md"
    try:
        gsd_surface_text = gsd_surface_skill.read_text(encoding="utf-8")
        check("gsd-codex-path", "${CODEX_HOME:-$HOME/.codex}" in gsd_surface_text and "CLAUDE_CONFIG_DIR" not in gsd_surface_text,
              "surface runtime points to CODEX_HOME")
    except OSError as exc:
        check("gsd-codex-path", False, str(exc))

    expected_gstack = {
        "gstack-browse", "gstack-design-consultation", "gstack-design-review",
        "gstack-plan-ceo-review", "gstack-qa", "gstack-qa-only", "gstack-review", "gstack-spec",
    }
    active_gstack = {path.name for path in (CODEX / "skills").glob("gstack-*") if path.is_symlink()}
    check("gstack-surface", active_gstack == expected_gstack,
          f"active={sorted(active_gstack)}, expected={sorted(expected_gstack)}")
    implicit = []
    for name in sorted(expected_gstack):
        metadata = CODEX / "skills" / name / "agents" / "openai.yaml"
        try:
            raw = metadata.read_text(encoding="utf-8")
            matches = re.findall(r"^\s*allow_implicit_invocation:\s*(true|false)\s*$", raw, re.MULTILINE)
            if matches != ["false"]:
                implicit.append(f"{name}:{matches}")
        except OSError as exc:
            implicit.append(f"{name}:{exc}")
    check("gstack-explicit-only", not implicit, ", ".join(implicit) or "all selected skills are explicit-only")
    gstack_config = GSTACK / "bin" / "gstack-config"
    try:
        proactive = subprocess.run([str(gstack_config), "get", "proactive"], text=True,
                                   capture_output=True, timeout=10)
        check("gstack-proactive", proactive.returncode == 0 and proactive.stdout.strip() == "false",
              f"exit={proactive.returncode}, value={proactive.stdout.strip()!r}")
    except (OSError, subprocess.TimeoutExpired) as exc:
        check("gstack-proactive", False, str(exc))

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
