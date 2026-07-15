# Summer Harness V3 Contract

Read this when changing Summer, its Skills, routing, migration or recovery.

## Invariants

1. Direct is the default; Summer persistence requires explicit user intent.
2. User lifecycle paths are Direct, Handoff Lite and Governed GSD. `Direct + Skill` is not a lifecycle.
3. Multi-Agent, multiple active Sessions, Phase/Wave/DAG or explicit GSD are hard GSD triggers.
4. Lite Handoff is sequential-only and can only promote explicitly to GSD.
5. `.agent/HANDOFF.md` is the only public recovery entry:≤4 KiB,≤5 `must_read`; Capsule≤32 KiB.
6. Git owns code; Lite Handoff owns the lightweight working set; `.planning/` owns heavy Workflow; Business Trust owns immutable Evidence/Execution/Review/Gate/TrustedUserInteraction/UserAcceptance/CompletionAuthorization/CancellationAuthorization; Evidence Store owns raw blobs; Projection owns no fact.
7. A fact type has one formal Writer. Trust records reference WorkRef/source digest and never mirror GSD Task status.
8. Engine remains the Apply/Query/Execute deep interface but is not a second Workflow owner.
9. Capability Router emits one primary +≤2 support Skills. Lite stores the compact canonical Plan in Handoff; every GSD routed activity stores immutable ActivitySkillPlan metadata and Worker Assignment references the same digest; Trust records only reference the digest. Skill `state_owner=none` and cannot widen authorization.
9a. Every SkillManifest, SkillPlan snapshot, normalized GateSpec set and Gate Policy digest used by a Receipt resolves to immutable canonical bytes in the Engine-written Contract Registry. Repeated content/object/plan/policy/gate-set digests and WorkRefs must satisfy exact canonical equality. Long-lived approval scope uses stable WorkIdentityRef (ProjectID+Backend+ExternalID), while Evidence/Gate freshness uses full revision-bound WorkRef. Activation uses candidate digest -> scope/project/WorkIdentity-matched TrustedUserInteractionReceipt -> independent ApprovedContractRecord; never put an approval digest back into its candidate. Every active Gate/permission Contract requires trusted User approval and updates create new versions.
10. Coordinator is the only actor authorized to advance governed state. Engine is the formal Handoff/Trust/Coordination writer; GSD Adapter is the formal `.planning/` writer. The shared Git common-dir lease/fencing epoch is revalidated before every Authority commit.
11. Worker uses isolated worktree/branch and returns Proposal through an untrusted inbox; Assignment, Proposal and accepted/rejected receipts are immutable Coordination records. Coordinator recalculates Git facts and Evidence freshness.
12. Completion requires declared Claim coverage, matching Evidence proof scope, fresh Execution/Review and an eligible CompletionAuthorization. GateReceipt binds SkillPlan, required Gate set and Policy digest; `failed` never authorizes. `limited` requires Policy allowance and a trusted Host user-interaction receipt that models/Coordinator cannot forge. Prose or `ActorRef.Role=user` is insufficient.
13. Evolution is inert until a trusted Host user-interaction receipt approves it; global changes require a second independent Host confirmation. ActorRef/model/Coordinator claims are not approval.
14. GUI/SQLite/Graph/Plugin/Adapter cannot write Authority directly.

## Routing

| Condition | Route | Authority |
|---|---|---|
| No explicit Summer intent | Direct | none |
| Sequential cross-session continuity | Handoff Lite | `.agent/HANDOFF.md` |
| Phase/Wave/DAG, parallel Agent, multiple active Sessions, explicit GSD | Governed GSD | `.planning/` |

Complexity may recommend activation but cannot authorize it. Unknown/ambiguous after activation defaults to Lite with an explanation; a later hard trigger requires explicit promotion.

## Current v0.1 Compatibility

The current binary implements Native v2 Objective start/save/resume/doctor and v1→v2 migration. This is legacy behavior:

- existing authorized Native work may resume;
- no new design may depend on Native Root Objective/WorkItem;
- v3 target commands must remain labelled planned;
- `mode=native` becomes `legacy-native` under the future migration surface;
- hand editing is forbidden.

## Native v2 -> v3 Migration

Migration must:

1. Diagnose source chain, Handoff, paths, secrets and one active source lifecycle.
2. Produce a deterministic zero-write dry-run.
3. Back up exact source bytes and metadata with a digest-bound manifest.
4. Map Objective working state to Lite or GSD without creating a second Task store.
5. Convert Objective/WorkItem-bound Evidence to WorkRef/source/workflow-digest binding.
6. Preflight capacity, path, adapter version and target Authority.
7. Back up target preimage bytes/metadata or require an absent target.
8. Under a Git common-dir source commit lock and target Coordinator lease, install a durable active migration fence before backup/target/Handoff write; unsupported or un-fenceable legacy Writers block migration.
9. Use pending journal + expected-preimage CAS to install target before switching Handoff; CAS failure must leave target untouched.
10. Fold/inspect both sides for semantic equivalence, then commit the fence as tombstone.
11. Keep migration-control records in an independent append-only Control chain excluded from Business Trust, Coordination and project-bound Contract digests. Before the first project-related v3 Authority mutation in any of those namespaces or Lite/GSD, write a pending marker and durable immutable rollback-close record bound to the intended exact successor/business digest; then commit the first write. Before closure, rollback requires unchanged target/Handoff/all three business digests plus a legal Control suffix, and restores exact source, target preimage and Handoff before closing the fence.

No GSD Adapter means `GSD_UNAVAILABLE`, not fallback. Native is never silently interpreted as Lite.

## Context Discipline

Persist goal, current WorkRef, verified results, one next action while non-terminal, blockers, consequential Decisions, Evidence and residual risks. A terminal Handoff clears next action and cites its Authorization. Do not copy transcript, chain-of-thought, source files, private raw logs or secrets into Handoff/Capsule.
