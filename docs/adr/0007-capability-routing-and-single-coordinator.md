---
status: accepted
---

# Capability Router 与单 Coordinator

Lifecycle Router 只选择 Lite 或 GSD；Capability Router 在每个 activity/stage 根据 WorkRef、acceptance、risk、artifact、failure signal、Evidence gap、Host 能力和 context budget 生成版本化 SkillPlan。

SkillPlan 恰有一个 primary、最多两个 supporting Skills；Lite 在 Handoff 保存紧凑 Plan，GSD 每个 routed activity 保存 immutable ActivitySkillPlan，Worker Assignment 只引用相同 digest。Skill `state_owner=none`，只能返回 Artifact、Proposal、Evidence draft、Finding 或 Gate result。`ask-matt` 不作为第二 Router。

多 Agent 时只有 Coordinator 能推进 `.planning`、Handoff 和 Trust acceptance。项目 lease 位于 Git common-dir，epoch 是每次 Authority commit 前必须重验的 fencing token；Worker 使用独立 worktree/branch，只交 immutable Proposal。Assignment/Proposal/Accepted|Rejected receipt 进入 append-only Coordination namespace，inbox 只是传输。Coordinator 重新计算 Git diff、allowed paths、dependency、Evidence freshness 和 CAS，不信任 Worker 自报。
