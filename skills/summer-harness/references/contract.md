# Summer Harness Contract

## Invariants

1. Direct is the default. Harness activation always requires explicit user intent.
2. `.agent/HANDOFF.md` is the only cross-session entry point.
3. A task has one lifecycle owner and one canonical ledger.
4. Native Summer tasks use `.agent/ledger/`; GSD tasks use `.planning/`.
5. The CLI is the only writer for `.agent/` lifecycle fields; Task + Handoff changes use a recoverable two-file transaction.
6. A session is disposable. Task, Decision, Fact, evidence, and validation survive it.
7. Facts are appended or invalidated, never silently rewritten.
8. Completion is a checked transition, not a prose claim.
9. Recovery context is bounded: at most 32 KiB, five must-read files, three recent decisions, and twelve valid facts.
10. Databases, dashboards, caches, and generated summaries are projections and may never become canonical.

## Routing

| User intent | Engine | Persistent state |
|---|---|---|
| Ordinary implementation, answer, review, fix | Direct | None |
| “保存交接 / 下个 session 继续” | Direct + Handoff | `.agent/HANDOFF.md` only |
| “使用 Summer Harness / 走 Harness” and bounded work | Summer native | `.agent/ledger/` + Handoff |
| Explicit Harness request and true multi-phase delivery | GSD | `.planning/` + pointer Handoff |
| Long unattended external runner | Dedicated runner only if explicitly requested | Runner-owned state + pointer Handoff |

Task size can justify recommending Harness, but can never silently activate it.

## State Machine

```text
active <-> blocked
   |          |
   +------> review ----> done
```

`done` requires acceptance criteria, validation bound to the current Task revision, no open blockers, and profile-specific gates. `high-risk` and `release` require an independent approved review bound to the current revision. `release` requires residual-risk acknowledgement when residual risks exist.

## Context Compaction

Promote only information that changes future work:

- Decision: a consequential choice and rejected alternative.
- Fact: an observed, sourced, confidence-rated statement.
- Task: outcome, acceptance criteria, state, next action, verification, and residual risk.

Do not persist chain-of-thought, exploratory chatter, duplicated source text, or stale TODO lists. Archive completed tasks in place; do not load them during normal restore.
