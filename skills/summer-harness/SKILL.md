---
name: summer-harness
description: Run an explicitly requested Summer Harness workflow using Handoff Lite for sequential continuity or Governed GSD for phase-based, parallel, or multi-session delivery. Use only when the user explicitly says “使用 Summer Harness”, “走 Harness”, or invokes `$summer-harness`; never auto-activate it for ordinary work.
---

# Summer Harness

Summer has one explicit Activation Gate and two persistent target backends. Do not repeat or delegate the lifecycle decision to another router.

## Current Transition State

The repository currently ships v0.1 Native v2 continuity commands. They are legacy compatibility, not the v3 target surface.

- Do not start a new Native lifecycle.
- An existing Native v2 project may resume already-authorized work, use the current `summer save` compatibility writer while no migration fence exists, run `summer doctor`, or wait for explicit v3 migration.
- Do not advertise `--lite`, `--gsd`, `route --explain`, Capability Router, GUI, or Gate commands as implemented until their milestone lands.
- Never hand-edit Native Ledger, HEAD, Snapshot, Handoff, or migration archives.

## Select The Target Backend Once

After explicit Summer authorization:

- Without an override, classify the goal with the Lifecycle Router and report reasons/hard triggers before creating state.
- Choose `Handoff Lite` only for one sequential workflow that needs cross-session continuity and has no Phase/Wave/DAG or parallel writer.
- Choose `Governed GSD` for Phase/Wave/DAG, two or more Agents, two or more active Sessions, or an explicit GSD request. `.planning/` is the Workflow authority.
- `Direct + Skill` remains Direct and is not another lifecycle.
- Lite may explicitly promote to GSD; GSD cannot silently fall back to Lite.

If the target command is not implemented yet, state the capability gap. For current heavy work, invoke the installed GSD workflow directly and use `$project-handoff` for the public pointer; do not emulate Governed GSD by creating another Ledger.

## Capability Routing

Lifecycle routing and Skill routing are separate:

1. Build a stage context from WorkRef/goal, acceptance, stage, artifact, risk, failure signals, Evidence gaps, allowed effects, Host capabilities, and context budget.
2. Select exactly one primary and at most two supporting Skills.
3. Record route reasons, Skill version/digest, expected Artifact and Evidence/Gates in the current Assignment or working set.
4. If more Skills are required, split the Assignment.

Matt Skills are narrow capabilities. Do not call `ask-matt` as a second router. No Skill may own lifecycle or write Handoff, `.planning`, or Trust Journal.

## Handoff Lite

Until the v3 Go writer exists, `$project-handoff` is the supported lightweight continuity path.

Target invariants:

- `.agent/HANDOFF.md` ≤4 KiB;
- `must_read`≤5, repo-relative and safe;
- Resume Capsule≤32 KiB;
- exactly one next action while non-terminal; completed/cancelled clears it with the matching Authorization;
- sequential Writer with revision/content CAS;
- parallel request returns a GSD route, not a merge into Handoff.

## Governed GSD

Let GSD own Requirement/Phase/Plan/Wave/Task in `.planning/`. Summer-owned records may reference a GSD WorkRef and snapshot digest but must not mirror Task status.

Multi-Agent rules:

- one Coordinator lease under Git common-dir;
- Worker uses isolated worktree/branch and bounded Assignment Capsule;
- Worker returns immutable Proposal and cannot write `.planning`, Handoff, or Trust Journal;
- Coordinator recalculates SHA/diff/paths/dependencies/Evidence freshness before accept;
- Coordinator authorizes governed advancement; Engine persists Handoff/Trust/Coordination and GSD Adapter persists `.planning/`;
- Authority writes are serial even when code work is parallel.

## Trusted Delivery

Machine-required completion needs observed process or accepted CI Evidence with matching proof scope. Evidence/Execution/Review/Gate bind WorkRef, workflow/source digest, tree digest, evidence-set, SkillPlan, Gate set and Policy digest. Contract Registry must resolve those digests to exact immutable bytes. Any binding change makes the old result stale.

Manual attestation may explain research or limited validation but cannot satisfy a machine-required Gate. Limited acceptance, cancellation, Evolution and Policy approval require a purpose/target-bound TrustedUserInteractionReceipt from a Host channel the model cannot forge. High-risk Review cannot be self-approved by the same contributor/session.

## Cross A Boundary

Before a new Session, GSD Phase boundary, long context, or Agent dispatch:

1. Save one Lite checkpoint or GSD pointer.
2. Verify Authority/digest and run available doctor checks.
3. On resume, read applicable `AGENTS.md`, `git status`, then the single Handoff.
4. Load only current WorkRef/Phase/Plan and at most five `must_read` files.

Read [references/contract.md](references/contract.md) when modifying Summer, defining migration, or resolving an Authority conflict.
