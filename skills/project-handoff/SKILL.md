---
name: project-handoff
description: Save or restore the single durable `.agent/HANDOFF.md` without activating a full Harness. Use for sequential Direct/Handoff Lite continuity, Governed GSD pointers, or legacy Native recovery/migration.
---

# Project Handoff

`.agent/HANDOFF.md` is the only public cross-session entry. This Skill does not activate Summer Harness or choose a heavy Workflow without explicit intent.

## Restore

1. Find the project root; read applicable `AGENTS.md` and `git status`.
2. Read Handoff only when non-idle or the user asks to continue.
3. Validate size, mode, revision/digest, source reference and safe `must_read` paths.
4. Route by mode:
   - `lite|direct`: restore the one sequential working set; `direct` is a legacy read alias and the target writer emits `lite`;
   - `gsd`: validate `.planning` snapshot digest and follow current Phase/Plan pointer;
   - `legacy-native|native`: use current `summer resume/doctor` only for already-authorized compatibility work, then follow the explicit migration plan when available.
5. Never reconstruct state from chat history, mtime or Agent confidence.

## Save Direct or Lite

Until the v3 Go Lite writer is implemented, use the existing helper only for a sequential snapshot:

```bash
python3 <summer-skill>/scripts/harnessctl.py --repo <root> handoff \
  --mode direct \
  --goal "<observable outcome>" \
  --done "<completed result>" \
  --next "<one action>" \
  --validation "<command and result>" \
  --must-read "<critical file>"
```

Target Lite invariants are revision CAS,≤4 KiB,≤5 safe `must_read`, one next action while non-terminal, terminal Authorization with no next action, and no parallel Writer. Do not fake these target guarantees if the legacy helper cannot provide them.

The target v3 Handoff also carries the compact canonical current SkillPlan (primary,≤2 supporting SkillRefs, strategy, Evidence/Gate digests and Plan digest). A legacy Direct snapshot has no such guarantee and must not claim route continuity.

## Save GSD Pointer

Use `mode=gsd` and a `.planning/STATE.md` source pointer. `.planning/` remains Workflow authority; Handoff stores only source path/digest, current WorkRef, next command and bounded summary.

GSD internal pause/resume artifacts are not a second public entry. Do not copy Requirement/Phase/Task state into Handoff.

## Legacy Native

Current Native v2 Handoff is a migration source, not a target backend. Use the Go CLI as the only compatibility writer for already-authorized work:

```bash
summer --repo <root> resume
summer --repo <root> doctor
summer --repo <root> save --done "<verified result>" --next "<one action>" --validation "<evidence>"
```

The `save` compatibility path is allowed only for work that was already authorized before v3 migration exists, and only when no migration/promotion fence is present. Do not start a new Native lifecycle. Never hand-edit Ledger, HEAD, Snapshot, Handoff or migration archive. Future v3 migration must be dry-run/backup/CAS/tombstone/rollback controlled.

## Content Limits

Never store transcript, chain-of-thought, source copies, secrets, large diffs or raw logs. Keep only the current goal, verified done summary, one next action, blockers, validation, last verified commit and at most five `must_read` pointers.
