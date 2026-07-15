---
name: adaptive-harness-router
description: Compatibility-only router for explaining Direct, Handoff Lite, Governed GSD, or migrating legacy Native setups. Use only when explicitly asked about routing or migration; never auto-activate Summer.
---

# Adaptive Harness Router

This compatibility Skill explains one route and one reason. It does not initialize files and is not part of the default installed surface.

1. Default to `Direct`.
2. Treat a narrow Skill as a Direct capability overlay, not a lifecycle.
3. Use `Handoff Lite` only for one sequential workflow that needs cross-session continuity.
4. Use `Governed GSD` for Phase/Wave/DAG, multiple Agents, multiple active Sessions, or explicit GSD intent.
5. If existing state is Native v1/v2, return `Legacy Native -> explicit migration`; never route new work into Native.

Authority:

- Direct: none.
- Handoff Lite: `.agent/HANDOFF.md`.
- Governed GSD: `.planning/`; Handoff is pointer only.
- Legacy Native: `.agent/ledger/` is read-only migration source after v3 switch.

Do not route through `ask-matt`, Superpowers, Super Dev, old Coding Agent Harness or Stellarlink. Skill/GUI/Worker cannot become an Authority.

```text
Route: <Direct | Handoff Lite | Governed GSD | Legacy Native migration>
Reason: <one sentence>
Authority: <none | .agent/HANDOFF.md | .planning | legacy .agent/ledger>
Hard trigger: <none | parallel_agents | multiple_active_sessions | phase_graph | dependency_wave | explicit_gsd | legacy_native>
```
