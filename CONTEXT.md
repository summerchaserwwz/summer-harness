# Summer Harness Domain

Summer Harness 是显式启用、local-first 的 Coding Agent 控制面。它不替代 Host 或 GSD：Direct 保持零状态，Lite 保存顺序 Handoff，GSD 拥有重型 Workflow，Summer 提供能力路由、一致性保护与可信交付。

## Language

**Activation Gate**:
判断用户是否明确启用 Summer 的外层边界。复杂度只能产生建议，不能自动授权状态写入。
_Avoid_: Auto hook, implicit harness

**Lifecycle Route**:
当前工作使用的三条路径之一：Direct、Handoff Lite、Governed GSD。
_Avoid_: Profile, workflow flavor

**Direct**:
默认工作路径；Summer 不创建状态、不扫描全仓、不启动进程。可以临时使用窄 Skill，但 Skill 不成为生命周期。
_Avoid_: Unmanaged mode

**Handoff Lite**:
一个顺序工作流的跨 Session 当前工作集。它不包含 Phase graph，不支持多个 Writer。
_Avoid_: Mini GSD, lightweight ledger

**Governed GSD**:
由 `.planning/` 拥有 Requirement、Phase、Plan、Wave 和 Task，Summer 在外部增加 Coordinator、SkillPlan、Evidence 和 Gate。
_Avoid_: Summer GSD copy, pointer-only GSD

**Workflow Authority**:
唯一有权表达“要做什么、拆成什么、当前做到哪”的 Store。Lite 是 Handoff；重型项目是 `.planning/`。
_Avoid_: Canonical database

**WorkRef**:
对 Lite action 或 GSD entity 的稳定引用，包含 backend、external id、source revision/digest；不复制外部状态。
_Avoid_: WorkItem, mirrored task

**Actor**:
发出命令或记录的身份，可以是 User、Coordinator、Worker、Reviewer 或 System。
_Avoid_: Account, operator

**Coordinator**:
唯一能推进 `.planning`、Handoff 与 Trust acceptance 的 Actor。通过 Git common-dir lease 和 epoch 保持单 Writer。
_Avoid_: Master agent, manager process

**Worker**:
在有界 Assignment 和独立 worktree/branch 中执行工作的 Actor。Worker 只能提交 Proposal。
_Avoid_: Canonical writer

**Assignment Capsule**:
Worker 的有界授权，包含 WorkRef、workflow digest、base SHA、allowed paths、SkillPlan、Acceptance、proof scope、must-read 和 lease epoch。
_Avoid_: Full project prompt, job ticket only

**Proposal**:
Worker 提交的 immutable 候选结果，包含 commit、diff claim、Evidence refs、deliverables、gaps 和 risks；Coordinator 必须重新验证。
_Avoid_: Progress update, trusted result

**Capability Router**:
根据阶段、风险、交付物、失败信号、Evidence gap、Host 能力和上下文预算选择 SkillPlan；它不选择生命周期。
_Avoid_: ask-matt, lifecycle router

**SkillPlan**:
一个 primary、最多两个 supporting Skills，以及 Agent strategy、Evidence/Gate plan、route reasons、版本与 digest。
_Avoid_: Prompt bundle, skill state

**Contract Registry**:
Engine 写入的 immutable/content-addressed SkillManifest、SkillPlan snapshot、GateSpec set 与 Gate Policy 原文存储，使历史 digest 可反查、可重评、可审计；Plan snapshot 只是历史合同，不拥有当前路由状态。
_Avoid_: Latest config only, hash without bytes

**Evidence**:
对可观察事实的 immutable 证明，同时表达捕获 Trust 和实际 Proof Scope。
_Avoid_: Validation prose, confidence score

**Execution**:
针对 WorkRef、workflow snapshot、代码树和 Evidence 集合的一次 immutable 交付声明。
_Avoid_: Process run, task state

**Review**:
Reviewer 对特定 Execution/tree/evidence-set 的 immutable 判定；高风险 Review 需要 contributor/session 分离。
_Avoid_: Feedback note, self approval

**GateReceipt**:
对 Claim coverage、Evidence、Review、production wiring 和 freshness 的 immutable evaluation，限定为 verified、limited 或 failed；它本身不直接完成 Workflow。
_Avoid_: Done flag, completion capability

**Completion Authorization**:
由可授权 GateReceipt 生成、只允许一次 exact previous→successor transition 的 immutable capability。Failed 永不授权；Limited 需要 Policy 允许和模型/Coordinator 无法伪造的 trusted Host user acceptance。
_Avoid_: Gate result, reusable approval

**Trust Journal**:
保存 Evidence、Execution、Review、GateReceipt、UserAcceptance、CompletionAuthorization、provenance 和 migration/checkpoint records 的 append-only Store；不拥有 GSD Task status。
_Avoid_: Workflow ledger, project database

**Delivery Coverage Matrix**:
由 typed records 派生的 criterion 对账视图，包含 implementation、Evidence trust/scope、Review、wiring、git/proposal state、gaps 和 risks。
_Avoid_: CSV source of truth

**Handoff**:
唯一公开、≤4 KiB 的恢复入口。Lite 模式下保存顺序工作集；GSD 模式下只保存 pointer/digest 和当前 WorkRef。
_Avoid_: Session transcript, full memory

**Resume Capsule**:
从当前 Authority 构建、≤32 KiB 的一次性恢复输入；最多引用五个 must-read。
_Avoid_: Full repository context

**Projection**:
由 Authority 派生、可删除重建的 Snapshot、SQLite、FTS、Graph 或 GUI View。
_Avoid_: Source of truth

**Evolution Candidate**:
由重复失败或 Review finding 产生的惰性改进建议；只有 User 批准后才能改变 Policy、Skill、AGENTS 或代码。
_Avoid_: Self-modification, learned rule

## Legacy Terms

**Native v2 / Root Objective / WorkItem / Canonical Ledger**:
只用于描述当前 v0.1 实现、历史记录和 v2→v3 migration。新设计不得把这些术语当作 v3 Workflow Authority。
