---
status: accepted
---

# 三条用户路径与一类事实一个 Owner

Summer Harness v3 只保留 Direct、Handoff Lite、Governed GSD 三条用户生命周期。`Direct + Skill` 是能力叠加，不是生命周期；Native v2 只保留兼容读取和显式迁移。

Lite 的顺序工作集由 `.agent/HANDOFF.md` 拥有；GSD Requirement/Phase/Plan/Wave/Task 由 `.planning/` 拥有；Git 拥有代码。Summer Business Trust Journal 只拥有 immutable Evidence/Execution/Review/Gate/TrustedUserInteraction/UserAcceptance/CompletionAuthorization/CancellationAuthorization 与 provenance；Coordination namespace 拥有 ActivitySkillPlan、Assignment、ProposalReceived 和 ingest receipts；Contract Registry 拥有可按 digest 反查的 SkillManifest/SkillPlan/Gate/Policy bytes；Promotion/Migration Control namespace 只拥有 cutover 控制记录。Handoff/Trust/Coordination/Contract 的正式 Writer 是 Engine，`.planning/` 的正式 Writer 是 GSD Adapter；Coordinator/Session 是授权调用者；GUI/SQLite 是 Projection。

选择这个模型是为了删除 Summer 与 GSD 的功能重叠，同时保留 M1 的 CAS、幂等、fsync、recovery 和 migration 工程资产。任何字段不得在两个可写 Authority 之间镜像。
