---
name: project-handoff
description: Save or restore the single durable `.agent/HANDOFF.md` without activating a full Harness. Use when the user asks to save a handoff, pause for another session, continue previous work, or restore project context. It supports Direct snapshots, Summer native projections, and GSD pointers.
---

# Project Handoff

Use `.agent/HANDOFF.md` as the only public cross-session entry. This Skill does not activate Summer Harness.

## Restore

1. Find the project root; read applicable `AGENTS.md` and `git status`.
2. Read `.agent/HANDOFF.md` only when it is non-idle or the user asks to continue.
3. Run `summer --repo <root> resume`, then open no more than its five `must_read` files.
4. For `native`, continue through the current Objective. For `gsd`, follow the returned backend pointer. For `direct`, continue directly.
5. On drift or lifecycle conflict, run `summer doctor` and fail closed. Never reconstruct state from chat history.

The Python helper may be used only for legacy v1 Direct/GSD pointers when the Go command cannot write that mode. It is not a Native v2 fallback and must never run after `.agent/ledger/HEAD` exists.

## Save Direct Or GSD State

Resolve the installed `summer-harness` Skill directory, then use its legacy helper only for these pointer modes:

```bash
python3 <summer-skill>/scripts/harnessctl.py --repo <root> handoff \
  --mode direct \
  --goal "<observable outcome>" \
  --done "<completed result>" \
  --next "<one action>" \
  --validation "<command and result>" \
  --must-read "<critical file>"
```

For GSD, use `--mode gsd --active-artifact .planning/STATE.md`; `.planning/` remains canonical. The public resume path is still this Skill and `.agent/HANDOFF.md`, not `$gsd-resume-work` directly.

## Save Native V2

Use `summer save`; Handoff is derived from the Canonical Ledger:

```bash
summer --repo <root> save --done "<result>" --next "<one action>" --validation "<evidence>"
summer --repo <root> doctor
```

Keep Handoff below 4 KiB, use no more than five repository-relative `must_read` files, and never store transcripts, chain-of-thought, source copies, secrets, or large diffs.
