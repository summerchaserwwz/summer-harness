---
name: summer-harness
description: Explicitly activated lightweight project workflow with durable Task, Decision, Fact, risk gates, bounded context restore, and one `.agent/HANDOFF.md`. Use only when the user explicitly asks to use Summer Harness or Harness for multi-session, auditable, complex, high-risk, research, or release work. Never auto-activate it for ordinary tasks.
---

# Summer Harness

Summer Harness is opt-in governance. Ordinary work stays Direct. Once invoked, use its CLI as the single writer for `.agent/` state and keep the product repository as the source of truth.

## Route Once

Choose one execution engine for the whole active task:

1. Use `native` for bounded multi-session work that needs durable decisions, facts, evidence, or risk gates.
2. Use `gsd` for genuinely multi-phase work where fresh-context planning and execution are the main need.
3. Do not activate this skill merely because a task is difficult. The user's explicit Harness request is required.

When `gsd` is chosen, `.planning/` is canonical. Initialize `.agent/` only to write a pointer Handoff; never duplicate GSD phase state into the Summer ledger.

## Start Native Work

Resolve the script path relative to this `SKILL.md`, then run:

```bash
python3 <skill-dir>/scripts/harnessctl.py init
python3 <skill-dir>/scripts/harnessctl.py start \
  --title "<short title>" \
  --goal "<observable outcome>" \
  --acceptance "<verifiable condition>" \
  --profile standard \
  --risk medium
```

Profiles:

- `standard`: acceptance criteria plus real validation.
- `research`: source-backed findings and a reproducible conclusion.
- `high-risk`: standard gates plus approved review.
- `release`: high-risk gates plus explicit acknowledgement of residual risks.

Write only consequential memory:

```bash
python3 <skill-dir>/scripts/harnessctl.py decision --title "..." --question "..." --chosen "..." --source "..."
python3 <skill-dir>/scripts/harnessctl.py fact --statement "..." --source "file:test-or-URL" --confidence high
python3 <skill-dir>/scripts/harnessctl.py checkpoint --done "..." --next "..." --validation "..." --must-read "path"
```

Facts are append-only. Invalidate a stale fact instead of editing it:

```bash
python3 <skill-dir>/scripts/harnessctl.py fact --invalidate fact_ID --reason "what changed"
```

## Use GSD as Backend

Use GSD only after selecting the GSD engine. Let the appropriate `$gsd-*` skill own `.planning/`, then maintain the single recovery pointer:

```bash
python3 <skill-dir>/scripts/harnessctl.py handoff \
  --mode gsd \
  --goal "<current milestone outcome>" \
  --next "<one concrete next action>" \
  --active-artifact ".planning/STATE.md" \
  --resume-command '$gsd-resume-work'
```

Never run a parallel native Summer Task for the same GSD task.

## Add Narrow Capabilities

Skills are capabilities, not lifecycle owners. Select the smallest relevant Matt skill directly:

- bug root cause: `diagnosing-bugs`
- architecture or module boundaries: `codebase-design`
- domain concepts: `domain-modeling`
- TDD: only when explicitly requested or justified by risk
- code review: at a review gate or when explicitly requested

Do not auto-call `ask-matt`; it is help/navigation, not a stronger router. Do not let a capability skill create another state system.

## Resume and Close

For a native Summer task, checkpoint at every session boundary, phase transition, or before context becomes unreliable:

```bash
python3 <skill-dir>/scripts/harnessctl.py checkpoint --done "..." --next "..." --validation "..."
python3 <skill-dir>/scripts/harnessctl.py doctor
```

For a GSD task, do not call `checkpoint`. Let GSD update `.planning/`, then refresh the pointer with `handoff --mode gsd ...` and run `doctor`. For Direct work, use `$project-handoff`.

On a new session, read only `.agent/HANDOFF.md`, run `resume`, then open no more than the listed `must_read` files. Do not scan the entire ledger or transcript unless the capsule is inconsistent.

If a native Handoff projection is damaged but the canonical Task is intact, run `repair-handoff`; it rebuilds the projection without advancing the Task revision. For GSD, refresh `handoff --mode gsd` after `.planning/STATE.md` changes.

Complete through the gate; never hand-edit status to `done`:

```bash
python3 <skill-dir>/scripts/harnessctl.py review --summary "..." --reviewer "<agent-or-session-id>" --approved --independent
python3 <skill-dir>/scripts/harnessctl.py complete --summary "..." --validation "..."
```

For the exact router and state invariants, read [references/contract.md](references/contract.md) only when changing the Harness itself or resolving state conflicts.
