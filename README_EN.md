# Summer Harness

[中文](README.md) | [English](README_EN.md)

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-Apache--2.0-34d399)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.1.0--dev-f59e0b)](docs/roadmap.md)

**Summer Harness v3 is an explicitly activated, local-first, GSD-backed, trust-centered control plane for coding agents.**

Small tasks remain Direct. A single sequential workflow can cross sessions through one lightweight Handoff. Phase/Wave/DAG work, multiple agents, or multiple active sessions belong in GSD. Summer adds only capability routing, consistency boundaries, trusted delivery, and bounded recovery.

It is not a model, a prompt bundle, a general issue tracker, a worker scheduler, or a second implementation of GSD.

> [!IMPORTANT]
> Summer Harness is currently a **`v0.1.0-dev` development preview / v3 architecture freeze**. GitHub HEAD implements Native v2 continuity, CAS, a transaction digest chain, recovery, and v1-to-v2 migration; those features are now legacy compatibility. M2 machine Evidence is in development but is not part of the current public HEAD or installable build. The Handoff Lite Go writer, Governed GSD Adapter, Capability Router, Trust Gate, GUI, Host Adapter, and installers remain roadmap work.

## Architecture at a glance

> The diagram describes the target v3 architecture. It is not a claim that every component has shipped.

![Summer Harness v3 workflow architecture](docs/diagrams/summer-harness-v3-workflow.svg)

[View/download the interactive Archify HTML source (open the downloaded file locally)](docs/diagrams/summer-harness-v3-workflow.html) · [View the diagram source JSON](docs/diagrams/summer-harness-v3-workflow.workflow.json)

There is one main path:

```text
User request
  → Activation Gate: not explicitly enabled → Direct → deliver
  → Summer explicitly enabled → Lifecycle Router: Handoff Lite / Governed GSD
  → Capability Router: select the smallest stage-specific SkillPlan
  → Host / Workers perform real work
  → Evidence → Review → Gate
  → deliver the result, or save one Handoff for bounded recovery
```

## Why this exists

Coding agents usually do not need more prompts. They need stronger engineering boundaries:

1. **Heavy workflows slow down small tasks.** Initializing a full harness for every question or local fix costs more than it returns.
2. **Chat is not reliable continuity.** Conversations are compressed, truncated, and polluted; a new session may not know the trusted state or the one next action.
3. **Multiple agents create split-brain state.** If workers can all edit plans, Handoffs, or completion state, there is no authoritative answer.
4. **Skill bundles accelerate context rot.** Loading a whole methodology at once allows routing and process text to crowd out the actual task.
5. **“Done” is often only prose.** Tests, reviews, the Git tree, the workflow revision, and the scope of the claim may not be bound together.

Summer does not make every task heavy. It provides the smallest reliable control plane only after the user explicitly asks for it.

## Three lifecycle routes

Every request has one lifecycle. Skills are capability overlays, not a fourth route.

| Route | Use it for | Workflow authority | State cost |
|---|---|---|---|
| **Direct** | Q&A, research, review, small fixes, routine development | none | zero Summer state |
| **Handoff Lite** | one sequential goal that must cross sessions | `.agent/HANDOFF.md` | one ≤4 KiB working set |
| **Governed GSD** | Phase/Wave/DAG, long roadmaps, multiple agents, multiple active sessions | `.planning/` | GSD workflow plus Summer governance |

Routing rules:

- Direct is always the default. Complexity may justify a suggestion, but it cannot authorize activation.
- `Direct + Skill` is still Direct.
- Handoff Lite supports a sequential writer, not a phase graph or parallel workflow writers.
- Multiple agents, multiple active sessions, Phase, Wave, or DAG are hard triggers for GSD.
- Lite can be explicitly promoted to GSD. Lite and GSD must never remain writable at the same time, and GSD cannot silently fall back to Lite.
- Risk is not workflow size. A small high-risk sequential task may remain Lite while using stricter Evidence, Review, and Gates.

## Five-layer architecture

```text
User / Codex / Claude
          │
          ▼
Explicit Activation Gate
          │
          ▼
Control Plane
Lifecycle Router / Capability Router / Coordinator
          │
          ▼
Workflow Plane
Handoff Lite XOR Governed GSD (.planning/)
          │
          ▼
Execution Plane
Host Workers / isolated worktrees / Git / CI
          │
          ▼
Trust Plane
Evidence → Execution → Review → GateReceipt
          │
          ▼
Continuity & Product Shell
Handoff / Resume Capsule / SQLite / Graph / on-demand GUI
```

### Control Plane

The Activation Gate keeps requests in Direct unless Summer was explicitly enabled. Only then does the Lifecycle Router select Lite or GSD, establish the single Coordinator, and choose the smallest SkillPlan for the current activity.

### Workflow Plane

Owns what should be done, how it is decomposed, and where the work currently stands. Handoff owns Lite. GSD `.planning/` owns heavy workflows. Summer does not mirror GSD Requirements, Phases, Plans, Waves, or Tasks.

### Execution Plane

Codex, Claude, GSD workers, Git, tests, and CI perform the real work. Summer does not own the model queue or copy the host's worker scheduler.

### Trust Plane

Binds Claims, Evidence, Executions, Reviews, GateReceipts, and Authorizations to the current WorkRef, workflow revision, Git tree, evidence set, and policy.

### Continuity and Product Shell

Provides the single Handoff, a ≤32 KiB Resume Capsule, Attention, search, graph views, and an on-demand GUI. SQLite, FTS, Graph, and GUI data are disposable projections, never authorities.

## Design principles

- **Direct-first:** ordinary requests cause zero Summer writes, zero repository-wide scans, and zero resident processes.
- **Explicit activation:** Summer runs only when the user explicitly says to use Summer Harness or invokes `$summer-harness`.
- **One lifecycle, one authority:** a goal has exactly one writable workflow authority at a time.
- **Skills are capabilities, not lifecycles:** a Skill cannot own or directly write Handoff, `.planning/`, or the Trust Journal.
- **Bounded continuity:** recovery consumes a bounded capsule rather than replaying chat or scanning all history.
- **Parallel execution, serialized authority:** code work may be parallel; workflow, Handoff, and trust acceptance are serialized.
- **Evidence over assertion:** handwritten “tests passed” prose cannot satisfy a machine-required Gate.
- **Disposable projections:** deleting the GUI database must not destroy project truth.
- **Human-approved evolution:** repeated failures can produce improvement candidates, but agents cannot silently rewrite policy, Skills, AGENTS files, or code.

## One owner for each kind of fact

Summer avoids a universal database. Each fact domain has one authority:

| Fact | Sole owner |
|---|---|
| code, commits, trees, diffs | Git |
| Lite working set | `.agent/HANDOFF.md` |
| GSD Requirements / Phases / Plans / Waves / Tasks | `.planning/` |
| Evidence / Execution / Review / Gate / Authorization | append-only Trust Journal |
| referenced SkillPlans / GateSpecs / Policy bytes | content-addressed Contract Registry |
| search, graphs, GUI, reports | rebuildable projections |

Summer uses a stable `WorkRef` to reference a Lite action or GSD entity instead of copying another title, progress field, and status. This prevents Handoff, GSD, GUI, and the Trust Journal from maintaining four competing versions of “the current task.”

## Capability routing by stage

The Activation Gate first decides whether the request stays Direct or enters Summer. Once Summer is active, the Lifecycle Router chooses only Lite or GSD. The Capability Router chooses what the current activity needs.

A target SkillPlan contains:

- exactly one primary Skill;
- at most two supporting Skills;
- an `inline`, `fresh`, or `parallel-wave` strategy;
- the expected Artifact;
- required Evidence and Gates;
- explainable route reasons;
- Skill version and content digest.

If more capabilities are needed, the work is split into another Assignment instead of loading a large prompt bundle. Matt Skills are narrow capabilities such as `grilling`, `domain-modeling`, `codebase-design`, `diagnosing-bugs`, `tdd`, and `code-review`. `ask-matt` is not a second router.

## Multi-agent consistency

Workers operate in isolated worktrees and branches. Each receives a bounded Assignment Capsule and returns an immutable Proposal.

Only the Coordinator may advance `.planning/`, Handoff, or Trust acceptance. During Proposal ingest it recalculates and checks:

- base/head SHA and the actual diff;
- changed paths and path authorization;
- dependencies, Wave readiness, and task overlap;
- workflow digest and fencing epoch;
- Evidence freshness and proof scope;
- reviewer independence.

The central rule is: **parallelize code writes; serialize authority writes.**

The initial design promises local consistency within one Git common-dir. It does not claim cross-machine distributed consensus.

## Trusted completion

Summer decomposes “done” into an auditable chain:

```text
Claim / Acceptance
        ↓
Evidence
        ↓
Execution
        ↓
Independent Review
        ↓
GateReceipt
        ↓
CompletionAuthorization
        ↓
Exact terminal transition
```

Evidence records two independent dimensions:

- **Capture trust:** observed process output, CI attestation, file digest, or manual explanation.
- **Proof scope:** what it actually proves—static, unit, integration, e2e, production wiring, or an external side effect.

A GateReceipt is only a `verified / limited / failed` evaluation. It does not own completion. A one-use CompletionAuthorization must bind the current WorkRef, workflow, tree, evidence set, SkillPlan, Gate Policy, and exact successor before a terminal transition is allowed.

Any binding change makes the old result stale. `failed` never authorizes. `limited` can continue only when policy permits it and the Host supplies an explicit user interaction that the model cannot forge.

## Preventing context rot

Summer deliberately avoids storing the whole conversation:

- Handoff ≤4 KiB;
- Resume Capsule ≤32 KiB;
- at most five safe, repository-relative `must_read` paths;
- GSD uses fresh context at Phase, Plan, and Wave boundaries;
- workers receive an Assignment Capsule instead of the full coordinator chat;
- a SkillPlan defaults to 1 primary +≤2 supporting Skills;
- raw logs and large artifacts remain in the Evidence Store, outside the prompt;
- Attention shows only blockers, drift, stale Evidence, pending Reviews/Proposals, and the one next action.

Handoff is therefore not a long-term memory database. It is a reliable boot sector for recovery.

## What Summer borrows—and what it does not

| Source | Adopted | Explicitly rejected |
|---|---|---|
| [GSD](https://github.com/open-gsd/gsd-core) | `.planning/`, Discuss/Plan/Execute/Verify, Phase/Wave, fresh context | copying the GSD workflow or creating a second task store |
| [Missions](https://github.com/flowing-water1/Missions) | Claim Coverage, proof scope, limited validation, production wiring, independent review | CSV as authority, sticky routing, multiple Handoffs |
| [Harness Anything](https://github.com/FairladyZ625/harness-anything) | provenance, immutable records, completion gates, rebuildable projections | global governance entities for every small change, an always-on heavy control plane |
| [Matt Skills](https://github.com/mattpocock/skills) | small, composable engineering capabilities | `ask-matt` as a second router, Skills owning lifecycle state |
| Summer v2 | Go deep kernel, CAS, idempotency, fsync, recovery, migration | Native Objective/WorkItem as the v3 workflow model |

The key division of responsibility is: **GSD owns heavy workflow, Matt Skills provide stage capabilities, the Host executes and schedules workers, and Summer owns activation, recovery, consistency, and trusted completion.**

## Implementation status

| Capability | Status | Notes |
|---|---|---|
| Native v2 `start/save/resume/doctor` | implemented | legacy compatibility; not recommended for new work |
| transaction digest chain, revision CAS, Engine/Ledger idempotency | implemented | current Go kernel |
| local cross-process write serialization | implemented | supported Unix-like platforms; not a distributed lock |
| Handoff/Snapshot rebuild and fault recovery | implemented | Native v2 continuity |
| v1-to-v2 dry-run / migration / rollback | implemented | current `summer migrate` scope |
| `Engine.Execute` / machine Evidence | M2 in progress | not in the current public HEAD or installable build |
| Handoff Lite Go writer / v3 migration | planned | M3/M4 |
| Lifecycle / Capability Router, Coordinator, Trust Gate | planned | M3 |
| Governed GSD Adapter | planned | M4 |
| on-demand GUI and SQLite/FTS/Graph projections | planned | M5 |
| Host adapters and Controlled Evolution | planned | M6 |
| `summer setup`, release binaries, Homebrew, signed desktop build | planned | M7 |

The v3 specifications define target commands, schemas, and invariants. That does not mean the current binary implements them.

## How to use Summer today

### 1. Ordinary work: stay Direct

Do not run Summer and do not create state files. Let the agent answer, research, review, fix, or build directly. Add one narrow Skill only when needed.

### 2. Sequential cross-session work: use Project Handoff

Use `$project-handoff` to save or restore the single `.agent/HANDOFF.md`. This does not activate the full Harness.

Handoff Lite is a target v3 backend. During the transition, `$project-handoff` can persist a sequential snapshot using the legacy `mode=direct` helper, but this does not provide the full CAS, SkillPlan, or terminal-authorization guarantees of the future Go writer.

### 3. Heavy or concurrent work: use GSD directly

When the work has Phases, Waves, a DAG, multiple agents, or multiple active sessions, use a separately installed GSD workflow and make `.planning/` the sole workflow authority. Handoff stores only a pointer, digest, current Phase/Plan, and resume command.

Until the Governed GSD Adapter ships, do not emulate GSD through another Summer Ledger. Use the installed GSD Skills for Discuss, Plan, Execute, and Verify.

### 4. Explicitly request Summer: invoke `$summer-harness`

The Skill explains the route, selects the target Lite or GSD backend, and either proceeds with implemented capabilities or reports the capability gap. It never upgrades ordinary work implicitly.

### 5. Existing Native v2 projects: compatibility only

```bash
summer resume
summer doctor
summer save \
  --done "<verified result>" \
  --next "<one action>" \
  --validation "<evidence>"
```

`summer save` is allowed only for previously authorized Native work that has not crossed a migration fence. Do not use `summer start` to create a new Native lifecycle. Do not hand-edit the Ledger, HEAD, Snapshot, Handoff, or migration archive.

Current `--validation` is checkpoint text, not machine Evidence, and current `doctor` checks Native continuity readability rather than the future full Authority/Trust/Adapter health surface.

## Installation

### Requirements

- Go 1.26 or newer;
- Git;
- optional: Codex or Claude Code;
- a separately installed compatible GSD Skill set for heavy workflows;
- Python 3 only if you use the transitional Project Handoff helper.

### Install the development preview with Go

There is no Git tag or formal GitHub Release yet. `@latest` installs the latest available repository revision, not a stable release.

```bash
go install github.com/summerchaserwwz/summer-harness/cmd/summer@latest
summer --version
```

Expected current output:

```text
summer 0.1.0-dev
```

Make sure the Go bin directory is in `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

### Build from source

```bash
git clone https://github.com/summerchaserwwz/summer-harness.git
cd summer-harness
mkdir -p bin
go build -o ./bin/summer ./cmd/summer
./bin/summer --version
```

### Optional: install the Codex Skills

Run from the repository root:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"

ln -sfn "$PWD/skills/project-handoff" \
  "${CODEX_HOME:-$HOME/.codex}/skills/project-handoff"

ln -sfn "$PWD/skills/summer-harness" \
  "${CODEX_HOME:-$HOME/.codex}/skills/summer-harness"
```

The recommended default surface contains only `project-handoff` and `summer-harness`. `adaptive-harness-router` is compatibility-only and should not be installed by default.

The following channels have not shipped:

- `summer setup codex|claude`;
- GitHub Release binaries, checksums, or SBOMs;
- a Homebrew formula;
- signed and notarized desktop applications.

## Current CLI surface

The current binary provides only the Native v2 compatibility surface:

```text
summer start <goal> [--next <text>] [--repo <path>] [--json]
summer save [--done <text>] [--next <text>] [--validation <text>] [...]
summer resume [--repo <path>] [--json]
summer migrate --dry-run [--repo <path>] [--json]
summer migrate [--repo <path>] [--json]
summer migrate --rollback [--repo <path>] [--json]
summer doctor [--repo <path>] [--json]
summer --version
```

`--lite`, `--gsd`, `route --explain`, `promote gsd`, `run --`, `check`, and `ui` are target v3 commands and are not implemented in the current release surface.

## Migration boundary

The current `summer migrate` implements v1-to-v2 only:

```bash
summer migrate --dry-run
summer migrate
summer resume
summer doctor
```

The target v2-to-v3 migration requires a zero-write dry run, byte-for-byte backup, semantic equivalence, a CAS Handoff switch, persistent tombstone, crash recovery, and rollback before the first v3 write. Until that milestone ships, manual Handoff/Ledger edits are not a valid substitute.

## Roadmap

| Milestone | Status | Focus |
|---|---|---|
| M0 | complete | v2 architecture baseline |
| M1 | complete | Go Engine/Ledger, continuity, v1-to-v2 migration |
| G0 | complete | v3 architecture, authority, and migration-contract freeze |
| M2 | in progress | machine Evidence, Execution, Review, freshness |
| M3 | planned | Lite/Capability Router, Coordinator, Gate/Authorization |
| M4 | planned | Governed GSD Adapter, promotion, v2-to-v3 migration |
| M5 | planned | on-demand GUI and rebuildable projections |
| M6 | planned | Host adapters and human-approved Evolution |
| M7 | planned | installation, releases, Homebrew, desktop distribution |
| M8 | planned | open-source release material, examples, and full release evidence |

See the [Delivery Roadmap](docs/roadmap.md) for exact boundaries, verification commands, and stop-if conditions.

## Development and verification

```bash
go test ./internal/...
go test -race ./...
go vet ./...
python3 -m unittest tests.test_harnessctl -q
python3 scripts/system_doctor.py
python3 scripts/check_architecture_contract.py
```

Validate the Archify workflow diagram:

```bash
node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" \
  validate workflow docs/diagrams/summer-harness-v3-workflow.workflow.json --json

node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" \
  check docs/diagrams/summer-harness-v3-workflow.html
```

## Design documents

- [v3 Product Specification](docs/product-spec-v3.md)
- [v3 System Architecture](docs/architecture-v3.md)
- [v3 Data Model](docs/data-model-v3.md)
- [Domain Language](CONTEXT.md)
- [Authority Matrix](docs/architecture-v3.md#authority-matrix)
- [Route Table](docs/architecture-v3.md#route-table)
- [Delivery Roadmap](docs/roadmap.md)
- [Threat Model](docs/threat-model.md)
- [Interactive v3 system diagram](docs/diagrams/summer-harness-v3.html)
- [Native v2 historical architecture](docs/architecture-v2.md)

Detailed architecture and security documentation is currently Chinese-first. Full bilingual documentation remains an M8 release goal.

## Explicit non-goals

- implicitly activating the Harness for ordinary requests;
- creating Native Objectives/WorkItems for new work;
- dual-writing GSD task state into Handoff and `.planning/`;
- allowing Skills, workers, GUI, SQLite, or plugins to write an authority directly;
- copying the host's worker scheduler into Summer;
- treating CSV or a projection as canonical state;
- letting an agent approve its own high-risk review;
- automatically modifying policy, Skills, AGENTS files, or code without user approval;
- presenting mocks, fixtures, dry runs, or prose as real integration, e2e, or external-side-effect Evidence.

## License

[Apache License 2.0](LICENSE)
