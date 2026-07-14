---
name: adaptive-harness-router
description: Lightweight compatibility router for choosing Direct, one narrow capability skill, Project Handoff, Summer native, or a GSD backend. Use when the user explicitly asks which workflow or harness route to choose, or when migrating an older harness setup. It never auto-activates Harness.
---

# Adaptive Harness Router

Return one route and one short reason. Do not initialize files while routing.

## Decision Order

1. Default to `Direct`.
2. If one specialist capability materially improves the task, choose `Direct + <skill>`.
3. If the only durable need is another session, choose `Direct + project-handoff`.
4. Only after explicit user authorization for Harness, choose `Summer native` for bounded persistent work.
5. Choose `GSD backend` only for explicit Harness/GSD work that is genuinely multi-phase and benefits from fresh-context execution.

Complexity, file count, duration, subagents, or risk may justify recommending a route, but never silently authorize Harness.

## Capability Map

- bug root cause: `diagnosing-bugs`
- architecture and module boundaries: `codebase-design`
- domain concepts: `domain-modeling`
- TDD: explicit request or risk-driven need
- review: explicit request or Summer risk gate
- product/design/QA: one selected gstack skill when its narrow capability fits
- GSD: phase lifecycle owner, never a capability add-on

`ask-matt` is manual help for the installed Matt subset, not an automatic or stronger lifecycle router.

## State Rule

There is exactly one lifecycle owner and one cross-session entry point:

- Direct: no state, or `.agent/HANDOFF.md` only.
- Summer native: `.agent/ledger/` canonical; Handoff derived.
- GSD: `.planning/` canonical; Handoff pointer only.

Do not route to Super Dev, Superpowers, old Coding Agent Harness, or Stellarlink Harness. Do not create parallel ledgers.

Output format:

```text
Route: <Direct | Direct + Skill | Direct + Handoff | Summer native | GSD backend>
Reason: <one sentence>
Persistent state: <none | .agent/HANDOFF.md | .agent/ledger | .planning>
```

