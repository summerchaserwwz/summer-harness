---
name: project-handoff
description: Save or restore the one durable `.agent/HANDOFF.md` for a project without activating a full workflow. Use when the user explicitly asks to pause, hand off, continue in another session, restore prior work, or preserve context. It supports Direct, Summer native, and GSD pointer modes.
---

# Project Handoff

Use one small repository-local file to cross session boundaries. This skill does not activate Summer Harness and does not create a task ledger unless one already exists.

## Restore

1. Find the project root and read applicable `AGENTS.md` plus `git status`.
2. If `.agent/HANDOFF.md` exists, read it before other workflow state.
3. Resolve the installed `summer-harness` skill directory and run its `scripts/harnessctl.py resume --repo <root>`.
4. Read only the returned `must_read` files and the canonical `source_path`.
5. If mode is `gsd`, continue through the named `$gsd-*` command; `.planning/` remains canonical.
6. If mode is `native`, use the active Summer task. If mode is `direct`, continue directly. If mode is `idle`, report that no work is active.

Fail closed when `resume` reports a digest mismatch. Run `doctor`; do not invent state from the chat transcript.

## Save Direct Work

Write a concise snapshot directly; this creates no Task ledger or Harness config:

```bash
python3 <summer-skill>/scripts/harnessctl.py --repo <root> handoff \
  --mode direct \
  --goal "<current observable outcome>" \
  --done "<completed result>" \
  --next "<one concrete next action>" \
  --validation "<command and result>" \
  --must-read "<critical file>"
```

Keep it below 4 KiB. Record at most five `must_read` paths. Do not copy conversation history, chain-of-thought, large diffs, or full design documents into Handoff.

## Save Managed Work

- Native Summer task: use `checkpoint`; it derives Handoff from the canonical Task.
- GSD task: use `handoff --mode gsd --active-artifact .planning/STATE.md`; never mirror phase state into `.agent/ledger/`.

Always run `doctor` after saving. The task is safely handed off only when `doctor` succeeds or reports only an understood non-blocking warning.
