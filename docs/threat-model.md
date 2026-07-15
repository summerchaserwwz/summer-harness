# Summer Harness v3 Threat Model

## 保护目标

1. Direct 请求不被隐式升级为持久 Workflow。
2. Lite Handoff 与 GSD `.planning/` 的 Authority 边界清楚且不可双写。
3. Trust Journal 的 Evidence/Execution/Review/GateReceipt 完整、不可静默改写。
4. Handoff 真实、有界、可恢复，不被旧 Session 或不安全路径投毒。
5. 多 Worker 不越过 Assignment、allowed paths、base SHA 和 Writer 权限。
6. Capability Router 不让 Skill 扩大授权或建立第二状态源。
7. GUI、SQLite、Projection、Plugin 和 Host Adapter 不成为绕过 Engine/Coordinator 的写路径。
8. Migration 不丢失 Native v2 bytes/semantics，也不允许旧 Writer 复活。
9. 开源发布不泄露 secret、完整会话、个人路径或敏感 Artifact。

## Trust Boundaries

```text
User explicit activation             高信任授权边界
Git code/tree                        代码事实
Lite Handoff                         顺序工作集 Authority
GSD .planning                        重型 Workflow Authority
Engine + Trust Journal               Trust/receipt Authority
Coordinator lease                    本地单 Writer 控制
Observed process Evidence             可证明 Summer 观察到本地执行
CI attestation                       取决于 CI identity/artifact retention
Worker Proposal                      不可信输入，必须重算
Skill / Plugin / web content         不可信建议或数据
GUI / SQLite / graph                 可重建 View，不可信写源
Local malicious process / OS root    超出本地 Harness 可完全防御范围
```

## Threats and Mitigations

### Implicit activation

风险：复杂任务、Skill 或 Hook 在用户未授权时创建 `.agent`/`.planning`、扫描仓库或启动进程。

措施：Activation Gate；默认 Direct；Capability Router 不能创建生命周期；无显式意图返回 `EXPLICIT_ACTIVATION_REQUIRED`。Direct 性能测试验证零 Summer 写入和零常驻进程。

### Dual Workflow authority

风险：Handoff、`.planning` 和 Trust Journal 同时保存可变 Phase/Task 状态，出现 split-brain。

措施：Authority Matrix 与 architecture contract checker；Lite Handoff 只支持单流；GSD `.planning` 独占重型 Workflow；Trust record 只引用 WorkRef/source digest；Worker/GUI/Skill 禁止写 Authority。

### Handoff poisoning and stale session overwrite

风险：攻击者或旧 Session 修改 Goal、next、source digest 或 must-read，引导恢复错误工作。

措施：4 KiB/5-file 上限、revision/content digest CAS、独立无自引用的 Lite working-set digest、原子 write/fsync/rename、安全 repo-relative path、symlink containment。Lite same-revision 冲突 fail-closed；GSD Handoff 与 `.planning` snapshot digest 不符时从 Authority 可证明重建，否则拒绝。

### Lite to GSD promotion split-brain

风险：出现多 Agent/Phase 硬触发后直接创建 `.planning/` 或先改 Handoff mode，崩溃时 Lite 与 GSD 同时可写。

措施：Git common-dir promotion fence、expected Lite digest、target preimage exact backup、staged GSD、pending marker、target CAS、Handoff CAS、`LitePromoted` record 与 committed tombstone。Active fence 同时阻止两侧普通 Writer；恢复只能证明唯一 successor 或精确恢复 preimage。

### GSD drift and cross-store crash

风险：`.planning` 在 Summer checkpoint 前后崩溃，或被绕过 Adapter 修改。

措施：Git common-dir Coordinator lease、staged exact successor、expected WorkflowSnapshot digest、ignored pending marker、唯一合法后继、fsync、`WorkflowCheckpointAccepted`、显式 reconcile。Terminal transition 的 GateReceipt 绑定 previous+successor digest 和 fencing epoch；禁止先标 done 后补 Gate。绕过修改标记 `UNACKNOWLEDGED_GSD_DRIFT`，在 reconcile 前不能获得 Verified Gate。

### Trust Journal tampering or torn writes

风险：Evidence/Review/Gate 半提交、手改、分叉或 idempotency 重放冲突。

措施：复用 M1 transaction chain、previous digest、single writer、observed revision CAS、idempotency key、transaction directory、fsync、atomic HEAD、pending marker 和 orphan policy。Trust Journal 不承担 GSD Task lifecycle。

### Fake or over-scoped completion evidence

风险：Agent 手写“测试通过”，用 unit/mock/dry-run 支持 integration/E2E/side-effect Claim，或使用旧代码树 Evidence。

措施：Evidence 分离 trust 与 proof scope；machine-required Gate 拒绝 manual attestation；记录 argv/exit/time/Git/tree/artifact；Coverage Matrix 检查 production wiring；Execution/Review/Gate 绑定 workflow/tree/evidence-set/SkillPlan/Gate set/Policy，变化即 stale。GateReceipt 只是 evaluation，CompletionAuthorization 只消费 exact successor。

### Forged limited user acceptance

风险：Coordinator 或模型把 `ActorRef.Role=user` 写入回执，伪造用户接受 validation gap。

措施：Engine 生成一次性 challenge；只有模型工具能力不可伪造的 trusted Host user-interaction channel 能返回 immutable TrustedUserInteractionReceipt。Receipt 绑定 purpose、scope、ProjectID/WorkIdentity、target/request digest、InteractionID 和 Host proof；长期 approval 绑定稳定 identity，Evidence/Gate freshness仍绑定完整 WorkRef。项目 BusinessTrustDigest 只投影匹配项目的 project/work receipt，global receipt排除。Host 无此边界时返回 `USER_ACCEPTANCE_UNAVAILABLE`，不允许 Coordinator 代填。Limited、cancellation、Evolution 和 Policy approval都引用同一通用 schema。

### Shell injection and repository escape

风险：Evidence Runner 把 argv 交给 shell，或 cwd/argument path 逃出仓库。

措施：默认 direct argv execution；显式 shell 必须独立授权并在 Receipt 标记；cwd、Artifact 和 file refs 做 realpath containment；environment allowlist；process start failure 是结构化 rejection。

### Secret leakage

风险：argv、environment、stdout/stderr、截图、Handoff 或公开 Journal 包含 token/key/个人路径。

措施：输入 preflight、stream redaction、value-free environment summary、大小/截断限制、private content-addressed Store、公开 record 只保存 digest/summary/retention。高置信 secret fail-closed；Release 扫描仓库、history 和 bundle。

### Coordinator split-brain

风险：不同 worktree 各自在 `.agent/runtime` 获取“唯一锁”，同时推进 `.planning` 或 Handoff。

措施：lease 位于 `git rev-parse --git-common-dir`；epoch 是 monotonic fencing token；每次 `.planning`、Handoff 或 Journal commit 前重验 owner/epoch/expiry/expected revision；renew 使用 CAS。只有 current lease holder 能接受 Proposal 和提交 Workflow checkpoint。状态写入串行，Worker code execution 可并行。

### Worker scope escape and confused deputy

风险：Worker 修改 canonical path、基于错误 SHA 工作、冒充 Assignment，或 Coordinator 信任 Worker 自报 diff。

措施：独立 worktree/branch、Assignment Capsule、base SHA、allowed paths、WorkRef/workflow digest、SkillPlan digest、lease epoch。Assignment、ProposalReceived 和 Accepted/Rejected receipt 进入 append-only Coordination namespace；common-dir inbox 只是 fsynced untrusted transport。Coordinator 从 Git 重新计算 diff/paths/SHA/overlap/dependency；Proposal immutable且 digest/idempotency-bound。命中 Handoff、`.planning/**` 或 Trust Journal 路径直接拒绝。

### Reviewer self-approval

风险：执行者换字符串身份给自己批准。

措施：Review 绑定 Actor、Session 和 contributors；高风险 Gate 要求不同 Actor 且不同 Session，并记录 independence strength。首发明确这是 provenance 约束，不宣传为密码学身份。

### Skill privilege expansion and supply chain

风险：Skill 建立自己的状态、写外部系统、加载大量上下文或含恶意指令。

措施：SkillManifest 固定 version/digest、effects、context cost、incompatibility、`state_owner=none`；默认最多 1 primary + 2 support；外部写仍需授权；未知或 digest 变化返回 `SKILL_UNAVAILABLE`/re-route。第三方内容只产生 Proposal/Finding。

### Evolution poisoning

风险：网页、日志或一次偶然失败被永久提炼为 Policy/Skill。

措施：Candidate 默认 inert，要求 source refs、occurrences、counterexamples、scope、risk、diff、validation 和 rollback；批准与 global second confirmation引用 purpose/target-bound TrustedUserInteractionReceipt，且二次确认使用不同 InteractionID。无可信用户通道时不可 apply；应用后要求 machine Evidence，失败进入 rolled_back。

### Projection and GUI split-brain

风险：SQLite/GUI 显示 Authority 不存在的状态或直接写缓存。

措施：Projection 记录 authority ref/revision/digest/projector version；写操作通过 Engine/Coordinator；不匹配时 refresh/rebuild；删除 Projection 必须能恢复相同 content digest。GUI 只绑定 loopback random port + short token + Origin/CSP/CSRF controls。

### Native v2 migration loss or writer revival

风险：迁移静默把 Native 当 Lite、删除原始 transaction、partial switch、rollback 覆盖新写入或旧 Python/Go Writer 复活。

措施：零写 dry-run；迁移与受支持 legacy Writer 共用 Git common-dir source commit lock，目标侧使用 Coordinator lease。Active fence 在 backup/target/Handoff write 前安装并阻止两侧普通 Writer；source/target 使用不可截断、不可脱敏的 exact backup manifest。Target 安装本身以 preimage revision/digest 做 CAS，失败前不触碰 target；随后 CAS Handoff switch并推进 committed tombstone。Migration control records 使用独立 Control chain，不进入 Business Trust/Coordination/Project Contract digest。任一项目相关 v3 Authority 首写在触碰 Authority 前先 durable-append 绑定 intended successor 的 `MigrationRollbackClosed`；其后崩溃只能完成/reconcile，不能 rollback。Rollback 仅在该 record 不存在且 target/Handoff/三个业务 digest未改变、Control suffix 合法时恢复 exact preimage。无 GSD Adapter返回 `GSD_UNAVAILABLE`。

### Host runaway cost or destructive action

风险：宿主 Worker 无限重试、消耗预算或执行破坏性外部动作。

措施：Summer 只发 Assignment/SkillPlan 和校验 Proposal，不启动自主模型循环；预算、超时、取消、破坏性命令和外部副作用继续由 Host 权限系统负责。

## Non-goals

- 不防御已控制本机内核、用户账号或磁盘的攻击者。
- 不把 local Evidence 宣传成远程可信执行证明。
- 首发不实现跨机器分布式共识、企业多租户 RBAC 或公共远程 daemon。
- 不允许为了安全幻觉破坏 Direct 零状态路径。

## Release Security Gates

- Activation/route、Handoff tamper、GSD drift、transaction crash、revision race、lease conflict、worker scope escape 和 migration rollback 有负向测试。
- Evidence redaction 使用固定向量，并证明原始 secret 未出现在 live sink、Trust Journal、Handoff、Git 或 Artifact metadata。
- GUI 通过 loopback、Origin、token、CSP、CSRF 和 arbitrary-file tests。
- release binary 提供 checksums、SBOM；桌面正式发布提供 signing/notarization Evidence。
- 仓库、Git history、release bundle 和示例不存在高置信 secret、开发者绝对路径或不可再分发资产。
