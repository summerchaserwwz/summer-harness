# Summer Harness V2 Contract

Read this only when changing the Harness, its Skills, or lifecycle recovery.

## Invariants

1. Direct is the default; Harness always requires explicit user intent.
2. `.agent/HANDOFF.md` is the only public cross-session entry.
3. A Root Objective has exactly one lifecycle owner and one canonical state source.
4. Native uses the append-only `.agent/ledger/` transaction chain. GSD uses `.planning/`.
5. Handoff, Snapshot, SQLite, GUI, search, graphs, summaries, and Git history are projections, caches, or secondary history; none can override canonical state.
6. Official writes pass through the Engine and one lifecycle lock. Expected revision, idempotency, validation, projection preflight, atomic commit, and fsync fail closed. A legacy Direct/Idle Handoff may transition to Native only through an explicit genesis `start`; v1 Native and GSD remain protected owners.
7. Session and Worker processes are disposable. Durable semantic state survives without storing transcripts or chain-of-thought.
8. Decision and Fact records are append-only once implemented; invalidation supersedes stale facts.
9. Handoff is at most 4 KiB with five `must_read` files. Resume Capsule is at most 32 KiB. Selection uses validity and current relevance, never age-only deletion.
10. Completion requires declared gates and real Evidence; a prose claim is never sufficient.

## Routing

| Explicit intent | Lifecycle owner | Canonical state |
|---|---|---|
| Ordinary answer, research, review, or implementation | Direct | None |
| Save or restore another session | Direct + Handoff | `.agent/HANDOFF.md` |
| “使用 Summer Harness / 走 Harness” for bounded durable work | Summer native | `.agent/ledger/` |
| Explicit Harness/GSD request for multi-phase fresh-context delivery | GSD | `.planning/` |

Task complexity may justify a recommendation but cannot authorize Harness. Capability Skills cannot become lifecycle owners. Host queues, gstack sessions, and GSD internal pause files are not public recovery sources.

## Current V0.1 Surface

```text
summer start <goal> [--next <action>]
summer save [checkpoint fields]
summer resume
summer doctor
summer migrate --dry-run
summer migrate
summer migrate --rollback
```

Current Objective checkpoint states are `active` and `blocked`. The target lifecycle later adds immutable Execution/Review and checked `review -> completed|cancelled` transitions. Do not advertise target commands as implemented.

CLI exit `3` means the canonical transaction committed but its projection requires repair. Callers must inspect `committed`, `projection`, and `code`; they must not retry it as an uncommitted write.

## Migration

V1 Native migration must:

1. Validate canonical legacy Handoff, Task bodies, Decision bodies, Fact order, references, secrets, paths, resource limits, and one nonterminal Objective.
2. Produce a zero-write dry-run and deterministic semantic/source summaries.
3. Back up exact v1 bytes with a bound manifest.
4. Import full history in one genesis transaction and fold it back for equivalence.
5. Switch Handoff with digest CAS.
6. Permit rollback only while genesis is the sole v2 transaction.
7. Recover a partial migration or rollback by journal, never by guessing.

Legacy Python Native writers must fail closed after a v2 footprint exists.

## Context Discipline

Persist only information that changes future work: goal, current next action, verified completion, blockers, consequential decisions, sourced facts, Evidence, and residual risk. Keep detailed canonical history in Ledger; keep only the current bounded working set in Handoff/Capsule. Never copy chat history, private reasoning, large logs, source files, or secrets into continuity state.
