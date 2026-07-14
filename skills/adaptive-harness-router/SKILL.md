---
name: adaptive-harness-router
description: Compatibility-only router for explaining or migrating older Harness setups into Direct, Project Handoff, Summer native, or GSD ownership. Use only when the user explicitly asks which route to choose or needs migration from an older router; it never auto-activates Harness and is not part of the default installed surface.
---

# Adaptive Harness Router

Return one route and one reason. Do not initialize files.

1. Default to `Direct`.
2. Use `Direct + Skill` for one explicitly useful specialist capability.
3. Use `Direct + Handoff` when persistence across sessions is the only durable need.
4. Use `Summer native` only after explicit Harness authorization.
5. Use `GSD backend` only after explicit Harness/GSD authorization for genuinely multi-phase fresh-context work.

State must have one owner:

- Direct: none, or `.agent/HANDOFF.md` only.
- Summer native: `.agent/ledger/` canonical; Handoff derived.
- GSD: `.planning/` canonical; Handoff pointer only.

Do not route through `ask-matt`, Superpowers, Super Dev, old Coding Agent Harness, or Stellarlink Harness. Matt is a capability collection, not a lifecycle. gstack is invoked only when the user explicitly names a concrete Skill; its own sessions or telemetry are never Summer state.

```text
Route: <Direct | Direct + Skill | Direct + Handoff | Summer native | GSD backend>
Reason: <one sentence>
Persistent state: <none | .agent/HANDOFF.md | .agent/ledger | .planning>
```
