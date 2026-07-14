---
name: summer-harness
description: Run an explicitly requested Summer Harness workflow with durable cross-session state, a canonical ledger, bounded restore, migration, and risk-shaped verification. Use only when the user explicitly says “使用 Summer Harness”, “走 Harness”, or invokes `$summer-harness`; never auto-activate it for ordinary work.
---

# Summer Harness

Keep exactly one lifecycle owner. Do not repeat the global Direct-first routing decision after this skill is explicitly invoked.

## Select The Backend Once

- Use `native` for a bounded Root Objective that needs durable checkpoints, auditability, or later multi-agent governance. `.agent/ledger/` is canonical.
- Use `gsd` only for genuinely multi-phase, fresh-context delivery. `.planning/` is canonical and `.agent/HANDOFF.md` is a pointer.
- Never run Native and GSD ledgers for the same objective.

## Run Native V2

Use the Go CLI as the only Native v2 writer:

```bash
summer start "<observable goal>" [--next "<first action>"]
summer save --done "<result>" --next "<one action>" --validation "<command and result>" --must-read "<path>"
summer resume
summer doctor
```

When `--next` is omitted, `start` uses the Goal as the first Next item. Keep Handoff below 4 KiB and `must_read` at five repository files or fewer. Use `--replace-done` and `--replace-validation` to compact checkpoints without deleting canonical history.

An explicit `start` may replace a legacy Direct/Idle Handoff. It must not replace GSD or v1 Native ownership. Exit code `3` means the canonical transaction committed but its Handoff/Snapshot projection needs repair; inspect `committed`, `projection`, and `code` instead of blindly retrying the write.

For a v1 Native project, migrate explicitly:

```bash
summer migrate --dry-run
summer migrate
summer resume
summer doctor
```

Use `summer migrate --rollback` only before any post-migration v2 transaction. Never hand-edit Ledger, HEAD, Snapshot, Handoff, or migration archives.

The current v0.1 surface implements Objective start/save, bounded resume, doctor, full v1 import, and crash-recoverable rollback. Decision, Fact, Evidence, Review, completion gates, Worker governance, Evolution, and GUI commands remain later milestones; do not emulate them with the legacy Python writer on a v2 project.

## Run GSD Backend

Let the selected `$gsd-*` workflow own `.planning/`. Use `$project-handoff` to save or restore the single public pointer. GSD pause/resume files are backend internals, not a second public recovery entry. Do not call GSD state-writing workflows while Native owns the lifecycle.

## Cross A Boundary

Before a new session, phase boundary, long context, or multi-agent dispatch:

1. Save one bounded checkpoint or GSD pointer.
2. Run `summer doctor`.
3. On resume, read applicable `AGENTS.md`, `git status`, then `summer resume`.
4. Open only returned `must_read` files unless diagnosis requires more.

Capability Skills may assist the work but cannot create another lifecycle or widen authorization. Use Matt or gstack only when explicitly selected under the global rules.

Read [references/contract.md](references/contract.md) only when modifying Summer Harness or resolving a lifecycle conflict.
