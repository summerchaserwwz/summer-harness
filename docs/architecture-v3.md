---
status: accepted
supersedes:
  - architecture-v2.md
---

# Summer Harness v3 Architecture

## 结论

Summer Harness v3 是一套显式启用、GSD-backed、trust-centered 的 Coding Agent 控制面，不是另一套通用项目管理器。

用户生命周期只保留三条路径：

1. **Direct**：默认路径，零 Summer 状态。
2. **Handoff Lite**：单工作流、顺序跨 Session 续接。
3. **Governed GSD**：多阶段、Phase/Wave/DAG、多 Agent 或多个活跃 Session。

Summer 不再为新工作建立 Native Root Objective、WorkItem、Phase 或 Roadmap。GSD `.planning/` 拥有重型 Workflow；Summer 专注于显式入口、统一恢复、Capability Router、单 Coordinator 一致性、machine Evidence、Review/Gate 和按需产品壳。

## 当前能力与目标能力

| 能力 | 当前 v0.1 | v3 目标 |
|---|---|---|
| Direct-first | 已实现为项目规则 | 保持 |
| Native v2 `start/save/resume/doctor` | 已实现，进入 legacy 兼容期 | 只读恢复或显式迁移，不再创建新 Native lifecycle |
| Handoff Lite | Direct helper 可用 | Go CLI 原生、CAS、顺序 Writer |
| GSD pointer | legacy helper 可用 | Governed GSD Adapter、checkpoint、drift detection |
| `Execute` / machine Evidence | M2 在途，不可宣称已交付 | WorkRef-bound Evidence/Execution/Review/Gate |
| Capability Router | 未实现 | 阶段级 SkillPlan、版本/digest、route explanation |
| Multi-Agent Guard | 未实现 | common-dir lease、Assignment Capsule、Proposal ingest |
| GUI / Evolution / Host Adapter | 未实现 | 按 Roadmap 分阶段交付 |

任何 README、Skill 或 UI 都必须区分这两列；目标命令存在于规格不等于当前二进制已经支持。

## Route Table

| 路径 | 进入条件 | Workflow authority | Writer 模型 | 退出/晋升 |
|---|---|---|---|---|
| Direct | 未明确启用 Summer | 无 | Host 正常工作 | 可保存 Lite Handoff |
| Handoff Lite | 只需顺序跨 Session；无 Phase/Wave/DAG；无并行 Writer | `.agent/HANDOFF.md` | 单一当前 Session，revision CAS | 只能显式晋升 GSD |
| Governed GSD | Phase/Wave/DAG、多 Agent、多个活跃 Session，或用户指定 GSD | `.planning/` | Coordinator 串行提交；Worker 只交 Proposal | Backend 在当前目标内冻结 |

`Direct + Skill` 是 Direct 的能力叠加，不是第四个生命周期。

### Activation Gate

复杂度可以产生建议，但不能授权状态写入。只有以下显式意图才能运行 Lifecycle Router：

- 用户说“使用 Summer Harness”或“走 Harness”；
- 用户调用 `$summer-harness`；
- 用户直接执行 `summer start`。

`summer start "<goal>"` 本身就是显式授权，随后 Lifecycle Router 自动选择 Lite 或 GSD并返回 reasons/hard triggers；`--lite`、`--gsd` 是用户 override。`summer route --explain` 只读预览相同决策，不创建状态。

普通问答、研究、审查、单点修复和常规开发不初始化 Summer、不扫描全仓、不启动 daemon。

### GSD 硬触发

满足任一项即选择 Governed GSD：

- 两个或更多 Agent 需要同时修改项目；
- 两个或更多活跃 Session 需要共享可变 Workflow 状态；
- 存在 Phase、Wave、依赖 DAG 或长期 Roadmap；
- 用户明确指定 GSD。

高风险本身改变 Trust/Gate profile，不必自动变成 GSD；一个顺序完成的高风险小改动仍可使用 Lite + Trust。

边界不清时选择 Lite，并返回可解释理由。出现硬触发后执行显式、单向 `Lite -> GSD` promotion；不得静默降级或同时激活两个 Workflow Backend。

### Lite -> GSD Promotion Saga

Promotion 不是复制文件或直接改 `mode`，而是一次受 fencing 保护的 Authority 切换：

1. 获取 Git common-dir Coordinator lease，并以 expected Lite revision/`working_set_digest` 安装 fsynced active promotion fence；所有 Lite Writer 从此拒绝保存。
2. 检查 `.planning/` target preimage。只允许 `absent|present_empty`；已有活跃或不同 Project 的 GSD 状态返回 `LIFECYCLE_CONFLICT`。
3. 原字节备份 Lite Handoff 和 target preimage；在非 Authority staging 中生成 GSD Project/Requirement/Phase context，计算 exact successor digest 并验证 semantic equivalence。
4. 写 pending marker，绑定 promotion id、Lite digest、target preimage digest、staged GSD digest 和 fencing epoch。
5. 在目标 Coordinator lease 下，以 target preimage 为 expected value 原子/CAS 安装 staged `.planning/`；CAS 失败前不得触碰 target。
6. 重验 lease、promotion fence 和 Lite expected digest，再 CAS 把 Handoff 切为 `gsd` pointer；active fence 保持不变，随后向 Promotion Control namespace 追加 immutable `LitePromoted` record并推进 committed tombstone。三者都 durable 后才解除 recovery mode，使任何旧 Lite Session 永久返回 stale/promotion-required。
7. fsync 并重新计算 `.planning` 与 Handoff digest；只有唯一 exact successor 可接受，随后清理 pending/staging。

崩溃时 active fence 同时阻止 Lite 与 GSD 普通 Writer。若 target 尚未安装，恢复原 Lite；若 target 已安装而 Handoff 未切换，recovery 只能在所有 expected digest 仍匹配时完成切换，或恢复 target preimage 与 Lite Handoff 后标记 rolled-back。任何无法证明的 partial state 都 fail-closed，绝不允许两个 Backend 同时可写。

## 五个平面

```text
User / Codex / Claude
          |
          v
Explicit Activation Gate
          |
          v
Control Plane
  Lifecycle Router / Capability Router / Coordinator
          |
          v
Workflow Plane
  Handoff Lite XOR Governed GSD (.planning/)
          |
          v
Execution Plane
  Host Workers / isolated worktrees / Git / CI
          |
          v
Trust Plane
  Evidence -> Execution -> Review -> GateReceipt
          |
          v
Continuity & Product Shell
  HANDOFF / Resume Capsule / SQLite / Graph / on-demand GUI
```

### Control Plane

决定是否进入 Summer、由哪个 Workflow Backend 工作、当前哪个 Actor 可以推进状态，以及每个阶段需要什么能力。它不能凭自己宣称交付完成。

### Workflow Plane

描述“要做什么、拆成什么、当前做到哪”。Lite 只维护一个有界工作集；GSD 维护 Requirement、Phase、Plan、Wave 和 Task。一次只能有一个 Active Workflow Backend。

### Execution Plane

Codex、Claude、GSD Worker、Git 和 CI 执行实际工作。Summer 不复制宿主的模型队列、并发、预算、取消、超时或重试系统。

### Trust Plane

描述“完成声明是否有可信证据”。Trust record 引用 Workflow WorkRef，但不复制 Workflow 状态，也不能自行扩展 Requirement。

### Continuity & Product Shell

提供统一 Handoff、Resume Capsule、Attention、Search、Graph 和 GUI。Projection 永远不能反写权威状态。

## Authority Matrix

一类事实只有一个 Owner 和一个正式 Writer。多个 Store 可以共存，但不能镜像可变字段。

| 事实 | Authority / Store | 正式 Writer | Revision / Digest | 恢复 | Drift 行为 |
|---|---|---|---|---|---|
| 代码、commit、tree | Git | Git plumbing（当前 branch owner 授权；Worker 只写自己的 branch） | commit/tree SHA | Git checkout/worktree | SHA 不符即 stale |
| Lite 当前工作集 | `.agent/HANDOFF.md` | Engine（当前 Lite Writer 授权） | handoff revision + working-set/content digest | 原子文件 + Git history | same-revision digest 冲突 fail-closed |
| GSD Requirement/Phase/Plan/Wave/Task | `.planning/` | GSD Adapter（Coordinator 授权） | WorkflowSnapshot digest | GSD 文件状态 + checkpoint | 未确认修改标记 `UNACKNOWLEDGED_GSD_DRIFT` |
| Evidence/Execution/Review/Gate/Completion/Cancellation Authorization | Summer Trust Journal | Engine；Coordinator 仅是授权调用者 | append-only record digest | transaction recovery | 绑定变化即 stale，不改写旧记录 |
| Trusted user interaction / acceptance authorization | Summer Trust Journal | Engine（仅在验证 trusted Host user interaction 后） | purpose/scope/project/WorkIdentity/challenge/target digest | Journal replay | global receipt不进入项目 Business Trust；ActorRef 不能代替授权 |
| SkillManifest/SkillPlan/Gate/Policy contract bytes | content-addressed Contract Registry | Engine | version + canonical content digest | exact bytes lookup | Plan snapshot只归档历史；Policy/privilege expansion需 trusted User approval |
| Promotion control records | Summer Journal 的 append-only Promotion Control namespace | Engine | independent control-chain digest | Control replay | 不拥有 Phase/Task；未知 suffix/双 successor fail-closed |
| Migration control records | Summer Journal 的 append-only Migration Control namespace | Engine | independent control-chain digest | Control replay | 不进入 Business Trust digest；未知 suffix fail-closed |
| raw stdout/stderr/Artifact | private Evidence Store | Evidence Module | content digest + retention | content-addressed lookup | 缺失标记 unavailable，不能伪造 |
| Coordinator lease | Git common-dir runtime | lease holder | monotonic fencing epoch + expiry | takeover protocol | 每次 Authority commit 前重验 epoch；过期即拒绝 |
| Assignment/ActivitySkillPlan/Proposal/ingest receipt | Summer Journal 的 append-only Coordination namespace | Engine；Coordinator 仅授权 issue/accept/reject | assignment/activity/proposal/content/receipt digest + idempotency key | Journal replay；common-dir inbox 仅作传输 | duplicate replay idempotent；digest 冲突拒绝 |
| Proposal inbox envelope | Git common-dir runtime | Worker | proposal id + content digest | 扫描未 receipt envelope；GC 仅在 durable receipt 后清理 | 不是 Authority，不决定 accepted state |
| Handoff GSD pointer | `.agent/HANDOFF.md` | Engine（Coordinator projector 授权） | planning snapshot digest | 从 `.planning/` 重建 | pointer 漂移重建或 fail-closed |
| SQLite/FTS/Graph/GUI | SQLite/FTS/cache | Projector | source cursor/digest | 删除重建 | 不匹配即 rebuild |
| Skill route result | Lite `Handoff.current_skill_plan` 或 GSD Coordination `ActivitySkillPlan` | Engine；Coordinator/当前 Lite Session 仅授权写入 | SkillPlan digest | 从当前 WorkRef/activity 读取 | Worker Assignment 引用同一 Plan；Skill/version 变化需新 Plan |

“Canonical Ledger”不再表示全项目唯一数据库。v3 使用更精确的 **Authority** 和 **Trust Journal**：Git、Lite Handoff、GSD Workflow 与 Trust records 各自只拥有自己的事实域。

## Handoff 与 Resume

Handoff 是“恢复启动扇区”，不是完整项目记忆。

### 目标 Schema

```text
schema
mode: idle | lite | gsd | legacy-direct | legacy-native
project_id
action_id                       # Lite stable action identity
goal
source_ref
source_revision
source_digest
working_set_digest             # Lite only; excludes WorkRef/SkillPlan/self digests
current_stage_or_work_ref
current_skill_plan              # Lite only; compact canonical 1+2 plan
action_state                    # Lite: active|blocked|in_review|completed|cancelled
done_summary
next_action                    # exactly one while non-terminal; empty when completed/cancelled
blockers
validation_summary
last_verified_commit
must_read                      # <= 5 repo-relative paths
resume_command
completion_authorization_digest # Lite terminal successor only
cancellation_authorization_digest # Lite cancelled successor only
content_digest
```

约束：

- 文件不超过 4 KiB；Resume Capsule 不超过 32 KiB。
- 不保存 transcript、思维链、源码副本、大日志或 secret。
- `must_read` 必须 repo-relative、存在且不能通过 symlink 越界。
- Lite Handoff 是顺序工作集的 authority；旧 Session 用 stale revision/working-set digest 保存时必须拒绝。Lite `WorkRef.SourceDigest` 绑定独立 working-set payload，不绑定包含自身的完整 Handoff digest。
- Lite 的 Handoff `source_ref/source_digest` 留空；它们只用于 GSD/legacy 外部 source pointer，不能与 `working_set_digest` 混用。
- Lite `current_skill_plan` 和 completion/cancellation authorization refs 都不进入 working-set digest，避免 Plan/Authorization→WorkRef/successor→Handoff 自引用；它们进入完整 Handoff content digest。Plan 保存 primary/supporting SkillRef、strategy、Evidence/Gate digests、reasons、context budget 和 Plan digest；容量 preflight 失败时必须拆工作，不能把完整 Plan 偷放到第二 Store。
- GSD Handoff 只保存 `.planning` 指针、当前 WorkRef、snapshot digest 和恢复命令；`.planning/` 始终获胜。
- `legacy-native` 只允许读取、诊断和迁移，不允许创建新的 lifecycle write。
- v1/v2 `mode=direct` 读取为 `legacy-direct`；v3 保存顺序 Handoff 时写 `mode=lite`。Direct 本身不持久化，一旦用户保存 Handoff 就进入 Handoff Lite。

### Resume 流程

```text
read AGENTS.md + git status
        |
        v
read .agent/HANDOFF.md when non-idle or requested
        |
        v
validate mode / revision / digest / safe paths
        |
        +-- Lite / legacy-direct -> build bounded capsule from Handoff
        |
        +-- GSD  -> inspect .planning snapshot -> current Phase/Plan capsule
        |
        +-- legacy-native -> MIGRATION_REQUIRED
```

失败不能从聊天、mtime 或“看起来最新”猜测状态。

## Deep Engine

外部保持三个入口：

```go
type Engine interface {
    Apply(ctx context.Context, command Command) (Receipt, error)
    Query(ctx context.Context, query Query) (View, error)
    Execute(ctx context.Context, spec RunSpec) (ExecutionReceipt, error)
}
```

Engine 是深 Module 和统一验证边界，但不是第二个 Workflow owner：

- `Apply` 提交 Summer 自有的 Trust/coordination/migration 记录，或通过 Adapter 接受一个已验证的 Workflow transition。
- `Query` 组合各 Authority 的只读 View。
- `Execute` 捕获真实 machine Evidence。

CLI、GUI、MCP、Skill 与 Host Adapter 不能绕过 Engine 写 Trust state。GSD Workflow transition 仍由 `.planning/` 表达，Summer 只保存 checkpoint/reference，不镜像 Task status。

## Capability Router

Lifecycle Router 选择 Backend；Capability Router 在每个 activity/stage 选择能力。两者不能合并，也不调用 `ask-matt` 形成嵌套路由。

### 输入

```text
WorkRef + acceptance
stage kind
artifact kind
risk profile
failure signals / blocker
Evidence gap
allowed effects
host capabilities
context budget
installed Skill manifests
```

### 输出

一个版本化 `SkillPlan`：

- 一个 primary Skill；
- 零到两个 supporting Skills；
- `inline | fresh | parallel-wave` Agent strategy；
- required gates 和 Evidence plan；
- route reasons；
- Skill version/content digest；
- unavailable/fallback semantics。

超过这个上限时拆分 Assignment，不一次性加载大型 Skill 套餐。

Lite 将当前 Plan 紧凑写入 Handoff。GSD 每个 routed activity 都先写 immutable `ActivitySkillPlan`：Discuss、Plan、Coordinator inline integration、Worker、fresh reviewer、Verify 和 Release 都适用；Worker Assignment 只能引用相同 Plan digest。这样 GSD Workflow 仍由 `.planning/` 拥有，而跨 Session 路由决定不会因“没有 Worker Assignment”丢失。

### 阶段矩阵

| 阶段/信号 | 能力 | 示例 Skill |
|---|---|---|
| 需求澄清 | requirements elicitation | `grilling`、`gsd-discuss-phase` |
| 领域建模 | ubiquitous language | `domain-modeling` |
| 架构设计 | deep module / boundary design | `codebase-design` |
| 诊断 | reproduce / root cause | `diagnosing-bugs` |
| 实现 | regression protection | `tdd` 或领域专用 Skill |
| 验证 | real runtime verification | browser / `playwright` / project verifier |
| 独立 Review | correctness/security/contract review | `code-review` + fresh reviewer |
| 发布 | packaging/release evidence | release-specific Skill/CI |

Skill 只能输出 Artifact、Proposal、Evidence draft、Finding 或 Gate result；`state_owner` 必须为 `none`。

## Governed GSD Adapter

GSD 擅长 `.planning` 持久状态、Discuss/Plan/Execute/Verify、fresh-context Agent 和 Wave。Summer 在其外部增加一致性和可信交付，而不是复制 GSD。

### Checkpoint Saga

跨 `.planning/` 和 Trust Journal 无法形成单文件事务，使用受控 Saga：

1. Coordinator 获取 Git common-dir lease/fencing epoch。
2. Adapter 读取并验证 expected WorkflowSnapshot digest。
3. 在非 Authority staging 目录生成完整候选 snapshot，校验允许路径并计算 exact successor digest；此时不得修改 `.planning/`。
4. 若为 terminal transition，使用当前 WorkRef/tree/evidence-set、SkillPlan digest、required Gate set digest、Gate Policy digest、expected previous digest 和 exact successor digest 评估 Gate，先追加 immutable GateReceipt。只有 `verified`，或 Gate contract 明确允许 limited 且存在绑定相同 Gate/Policy/digests/gaps/risks 的 trusted Host user-interaction UserAcceptanceReceipt，才能再生成 immutable CompletionAuthorization；`failed` 永不授权。
5. 写 fsynced pending marker，记录 operation id、previous/successor digest、fencing epoch 和 CompletionAuthorization digest。
6. Authority commit 前重新读取 common-dir lease，要求 owner/epoch 当前且未过期；再 CAS 验证 `.planning` 仍是 expected previous digest。
7. 原子安装或按可恢复 manifest 安装 staged snapshot；目标 snapshot 必须包含 authorization reference（terminal 时）。
8. fsync 后重新计算 Authority digest；只接受与 staged successor 完全一致的唯一后继。
9. Summer Journal 追加 `WorkflowCheckpointAccepted(previous -> successor, fencing epoch, authorization digest)`。
10. 清理 pending/staging并投影 Handoff。

崩溃后只在 pending marker、current fencing epoch、authorization（如需要）与唯一后继精确匹配时恢复接受；其他情况 fail-closed。绕过 Adapter 的直接 GSD 写入可以保留在磁盘，但在显式 reconcile 前不能获得 Summer Verified Gate。

Terminal transition 不允许“先把 `.planning` 标 done，再补 Gate”。GateReceipt 是 evaluation，CompletionAuthorization 才是 exact transition capability。Authorization 同时绑定 GateReceipt、SkillPlan/required Gate set/Policy、trusted User acceptance（如需要）、previous digest、staged successor digest、tree/evidence-set 和 fencing epoch。任何不匹配都会拒绝；成功 checkpoint 消费 Authorization 后，它只作为历史完成依据。

### Lite Terminal Checkpoint

Lite completion 由 M3 的 Handoff CAS 实现，不等待 GSD Adapter：

1. 以当前 revision/`working_set_digest` 构造 terminal working-set successor：`action_state=completed`、`next_action` 为空，并计算不含 Authorization/self digest 的 exact successor working-set digest。
2. GateReceipt 和 CompletionAuthorization 绑定 expected current working-set digest 与 authorized terminal working-set digest。
3. 写 pending marker；重验 Lite lease/revision、Gate/Policy/User proof freshness后，以 CAS 原子安装包含 `completion_authorization_digest` 的 Handoff。
4. fsync 后重算 working-set/content digest，并由 Engine 追加 `LiteCheckpointAccepted(previous -> successor, authorization digest)`；崩溃恢复只接受 expected previous 或唯一 authorized successor。

Authorization 绑定 working-set successor 而不是完整 Handoff ContentDigest，避免 successor 包含 Authorization digest产生自引用。成功后 Authorization 只能作为历史依据，不能用于下一 action。

Cancellation 使用独立 `CancellationAuthorization`：trusted Host 用户交互和 cancellation Policy 绑定 exact cancelled successor，Handoff 写 `cancellation_authorization_digest`。它不产生 Completion Claim，也不能复用 CompletionAuthorization。

## Multi-Agent Consistency

### Coordinator lease

锁必须基于：

```text
git rev-parse --git-common-dir
<GIT_COMMON_DIR>/summer/<project-id>/owner.lock
<GIT_COMMON_DIR>/summer/<project-id>/inbox/
```

不能把锁只放在单个 worktree 的 `.agent/runtime/`，否则不同 worktree 会各自认为自己是唯一 Writer。

### Assignment Capsule

Worker 只获得有界输入：

```text
assignment_id
WorkRef
workflow checkpoint digest
base SHA
branch/worktree
allowed paths
acceptance
SkillPlan
required proof scope
must_read
lease/epoch
```

Worker 不读取完整主对话，也不能写 `.agent/HANDOFF.md`、`.planning/**` 或 Trust Journal。

### Proposal ingest

Proposal 包含 head commit、claimed paths、diff digest、Evidence refs、deliverables、known gaps 和 residual risks。Coordinator 必须从 Git 重新计算并检查：

- lease/epoch 与 expected checkpoint；
- base/head SHA；
- changed paths 与 overlap；
- dependency/wave readiness；
- Evidence freshness；
- Reviewer independence 和 Gate requirements。

Assignment issuance 和 Proposal acceptance/rejection 是 append-only Coordination records。Worker 写入 common-dir inbox 的 Proposal envelope 只是 untrusted transport；Coordinator 校验后先把完整 Proposal（含 deliverables、Evidence refs、gaps、risks）写成 digest-bound immutable `ProposalReceived` record，再追加 `ProposalAccepted|ProposalRejected` receipt。只有两个 durable records完成后才能清理 inbox。状态 `issued -> submitted -> accepted|rejected|expired` 由 Journal replay 派生；相同 ProposalID+digest 重放幂等，不同 digest 冲突拒绝。

每次 `.planning`、Handoff 或 Journal Authority commit 前都必须重新读取 lease，验证当前 owner、未过期 fencing epoch 和 expected revision。Lease renew 使用 owner+epoch CAS；本地单调时钟决定 TTL，持久 wall-clock 只用于诊断。旧 Coordinator 在 lease 过期或更高 epoch takeover 后不能继续提交。

代码可以并行，Authority 状态更新必须串行。首发只保证同一 Git common-dir 下的本地并发；跨机器需要未来远程 Coordinator/distributed lease，不能宣传为已解决。

## Trust Plane

```text
Claim / Acceptance
        |
        v
Evidence (trust + proof scope)
        |
        v
Execution (WorkRef + workflow/tree/evidence-set digests)
        |
        v
Review (independent verdict)
        |
        v
GateReceipt: verified | limited | failed
        |
        v
CompletionAuthorization (only policy-authorized result)
```

Trust 等级：

```text
observed_process > ci_attestation > file_digest
> external_reference > manual_attestation
```

Proof scope 是正交维度：

```text
static | unit | integration | e2e
| production_wiring | external_side_effect
```

低等级 Evidence 不能支持高等级 Claim。`limited` 必须记录 validation gap、manual reproduction 和 residual risk；除非 Gate contract 明确允许且 User 接受，否则不能满足 machine-required Gate。

代码树、Workflow snapshot、WorkRef revision 或 Evidence 集合变化后，尚未用于 transition 的旧 Execution/Review/GateReceipt 自动 stale。

GateReceipt 只做 evaluation，并绑定 SkillPlan、required Gate set 与实际 Gate Policy digest。所有 digest 必须能从 immutable/content-addressed Contract Registry 反查 exact SkillManifest、SkillPlan snapshot、GateSpec set 与 Policy bytes；Lite/GSD Plan 在首次被 Trust record 引用前归档，snapshot 只保存历史合同，不成为第二个可变路由 Authority。Contract activation 使用 candidate→trusted receipt→ApprovedContractRecord 两阶段，不把 approval digest 写回 candidate；candidate/receipt/approval 的 scope/project/WorkIdentity 必须完全一致，GateReceipt 只能引用 global、同 Project 或与当前 `WorkRef.Identity()` 相等的 Approval。重复 content/object/plan/policy/gate-set digest 和 WorkRef 必须满足 canonical 等式。所有 active Gate/permission Contract 都要求 trusted-user approval；旧 contract 不覆盖。`failed` 永不生成 CompletionAuthorization；`limited` 只有在 Gate contract 的 `limited_allowed=true` 且存在模型/Coordinator 无法伪造的 trusted Host user-interaction UserAcceptanceReceipt 时才可授权；Host 不提供该边界时返回 `USER_ACCEPTANCE_UNAVAILABLE`。`verified` 可按同一 Policy 生成 Authorization。Coordinator 必须证明当前 Workflow digest 与 Authorization 的 expected digest 相同，再提交引用 Authorization digest 的 GSD/Lite completion checkpoint。`WorkflowCheckpointAccepted` 或 `LiteCheckpointAccepted` 记录 `previous_digest -> new_digest` 和 `completion_authorization_digest`。精确 checkpoint 消费后，Authorization 作为历史依据保留，不能授权后续工作。

## Delivery Coverage Matrix

Missions 的价值是 Claim/Evidence 闭环，不是 CSV 格式。v3 使用 typed records 派生：

| 字段 | 含义 |
|---|---|
| criterion | 可验收声明 |
| work_ref | Lite action 或 GSD Task reference |
| owner | 当前 Assignment owner |
| implementation | not-started / active / delivered |
| evidence_trust | Evidence 捕获可信度 |
| proof_scope | 实际证明范围 |
| review | pending / accepted / rejected / limited |
| production_wiring | not-required / missing / verified / limited |
| git_proposal_state | uncommitted / proposed / accepted |
| gap_risk | validation gap 与残余风险 |

CSV 只允许单向导入、导出或 GUI 表格 Projection；不得双向同步或成为第三个 Writer。

## Native v2 -> v3 Migration Contract

现有 v2 transaction chain 是不可丢失的历史与工程资产。v3 不继续扩张其 Root Objective/WorkItem lifecycle，但必须兼容读取并显式迁移。

### 迁移目标

| v2 内容 | `--to lite` | `--to gsd` |
|---|---|---|
| 当前 Objective goal/done/next/blockers/validation | 写入 Lite Handoff working set | 映射到 GSD Project/Requirement/current Phase context |
| Acceptance | Lite validation/next 摘要；超限则拒绝 | GSD Requirement/Plan |
| Decision/Fact 历史 | 只读 legacy history；未来可作为 Trust source ref | 同左，不复制成 GSD Task |
| M2 Evidence | 迁移为 WorkRef-bound Trust record | 绑定 GSD WorkRef + snapshot digest |
| v2 transaction bytes | 原字节 archive + immutable audit | 同左 |
| Root Objective/WorkItem status | 不继续写 | 不镜像；由 `.planning` 拥有新 Workflow 状态 |

### Legacy CLI 行为

| 命令 | 发现 Native v2 footprint 时的 v3 目标行为 |
|---|---|
| `summer resume` | 只读生成 bounded legacy Capsule，明确返回 `migration_required:true`；不得产生新 transaction |
| `summer doctor` | 验证 transaction chain/Handoff/migration journal，并报告可迁移目标 |
| `summer start` | 返回 `MIGRATION_REQUIRED`，不得覆盖或静默转成 Lite |
| `summer save` | 返回 `MIGRATION_REQUIRED`；当前 v0.1 兼容 Writer 只用于迁移功能交付前的既有授权在途工作 |
| `summer migrate native-v2 --to lite|gsd --dry-run` | 零写输出 mapping、风险、容量和 Adapter 检查 |
| `summer migrate native-v2 --to lite|gsd` | 执行 backup、target write、双读验证、CAS switch、tombstone |
| `summer migrate native-v2 --rollback` | 仅首个 v3 write 前恢复原字节与 Handoff |

### 必要步骤

1. `doctor` 与零写 dry-run；确认所有受支持 Go/Python legacy Writer 使用 Git common-dir source commit lock、识别 migration fence，并在最终 HEAD/Handoff commit 前于同一临界区重验。任何可能写 source footprint 的旧二进制无法围栏时拒绝迁移。
2. 获取 source commit lock 和目标 Coordinator lease/fencing epoch；重新读取 source head 与 target preimage。迁移与 legacy Writer 共用该 lock，目标 Writer 受 lease fencing。
3. 在 lock/lease 内、任何 backup/target/Handoff write 前，于 Git common-dir 原子安装并 fsync persistent active migration fence 和 source fail-closed marker，绑定 migration id、source head、target preimage digest、target epoch。Active fence 阻止 legacy Writer 和所有非 migration-recovery 的 Lite/GSD/Business Trust/Coordination/project-bound Contract write。
4. 在稳定快照上备份 source exact bytes/metadata/symlink/Handoff。检查 target preimage 并稳定区分 `absent|present_empty|adopted`；`present_empty` 也备份目录 metadata，`adopted` 只允许同 Project 显式确认。备份必须 exact、不可截断/脱敏；其他活跃 target 返回 `LIFECYCLE_CONFLICT`。
5. 生成 deterministic semantic mapping report，并对目标 Authority 做容量、路径、secret 和 Adapter version preflight；写 fsynced pending journal，在 staging 生成目标并计算 exact target digest。
6. 在目标 lease 下，以 preimage revision/digest 为 expected value 原子/CAS 安装 staged target；CAS 失败前不得触碰 target。随后 fold/inspect 双读并验证 semantic equivalence。
7. Authority/Handoff commit 前再次重验 common-dir fence、source head、target lease/epoch 和 exact installed target；CAS 切换 Handoff mode/source digest。
8. fsync 并验证 source archive、target、Handoff 和初始 Business Trust/Coordination/Project Contract digest 后，将 fence 推进为 `committed` tombstone。Migration control records 使用独立 Control chain，不推进这些业务 digest。Tombstone 永久拒绝 legacy writer；若在 cutover 后、commit marker 前崩溃，active fence 仍阻止两侧普通 Writer，recovery 根据 exact digests 完成唯一 commit 或恢复 preimage。
9. committed 后尚未发生项目相关 v3 写入时仍可 rollback。每个 Lite/GSD/Business Trust/Coordination/project-bound Contract Writer 在首次 Authority write 前必须检查 tombstone，写 first-write pending marker，并在触碰 Authority 前先追加/fsync immutable `MigrationRollbackClosed`，绑定 intended exact successor/business record digest 和三个 previous namespace digest；随后才提交该写入。若 close durable 后崩溃，rollback 仍永久关闭，recovery 只能完成或 reconcile 该首写，不能回退迁移。全局、未绑定 Project/WorkIdentity 的 Contract 对象不关闭该项目 rollback。
10. Rollback 要求不存在 close record，且 target、Handoff、Business Trust、Coordination、Project Contract digest仍精确等于 MigrationReceipt 初始摘要，Migration Control chain 只包含该状态机允许的 exact suffix。依次恢复 source、target preimage和 Handoff，验证 exact bytes/digests 后最后把 fence 标记 `rolled_back`；只有 `absent` target 可删除，`present_empty|adopted` 必须恢复原 metadata/bytes。

`native` 不得静默解释为 `lite`。无 GSD Adapter 时 `--to gsd` 返回 `GSD_UNAVAILABLE`，不能 fallback。partial migration、多个合法后继、跨 Project Handoff、未备份的 target preimage、GSD digest 漂移、无法共享 source lock/fence、target preimage CAS 失败或 rollback 已关闭都 fail-closed。

## Failure Taxonomy

至少定义并稳定输出：

```text
EXPLICIT_ACTIVATION_REQUIRED
LIFECYCLE_CONFLICT
MIGRATION_REQUIRED
GSD_UNAVAILABLE
HANDOFF_DRIFT
UNSAFE_HANDOFF_REFERENCE
UNACKNOWLEDGED_GSD_DRIFT
COORDINATOR_LEASE_CONFLICT
REVISION_CONFLICT
STALE_PROPOSAL
CANONICAL_PATH_VIOLATION
SKILL_UNAVAILABLE
EVIDENCE_MISSING
EVIDENCE_STALE
REVIEW_NOT_INDEPENDENT
USER_ACCEPTANCE_UNAVAILABLE
GATE_POLICY_STALE
PROMOTION_CONFLICT
MIGRATION_ROLLBACK_CLOSED
GATE_FAILED
PROJECTION_DRIFT
```

## Context-Rot Controls

- Handoff ≤4 KiB；Resume Capsule ≤32 KiB。
- 新 Session 只读取当前 Handoff、当前 GSD Phase/Plan 和最多五个 must-read。
- GSD 在 Phase/Plan/Wave 边界使用 fresh context。
- Worker 只拿 Assignment Capsule，不拿主对话历史。
- Skill progressive disclosure：每个阶段只加载 SkillPlan 所需内容。
- raw logs/Artifact 留在 Evidence Store，不进入 Prompt。
- Attention 只显示 blocker、stale Evidence、pending Review/Proposal、drift 和唯一下一步。

## Source Adoption Boundaries

- **GSD**：采用 `.planning`、Discuss/Plan/Execute/Verify、Phase/Wave、fresh context；不复制其 Workflow。
- **Missions**：采用 Claim Coverage、proof scope、limited validation、production wiring、independent review、gap follow-up；不采用 CSV authority、sticky router 或多 Handoff。
- **Harness Anything**：采用 provenance、immutable records、completion gates、rebuildable projection；不采用常驻重型控制面或为小改动建立全域实体。
- **Matt Skills**：采用小而组合式的工程能力；`ask-matt` 不成为生命周期 Router。
- **Summer M1**：保留 Go deep kernel、CAS、idempotency、fsync、recovery 和 migration；改变其事实所有权范围。

## Non-goals

- 不实现 Worker 模型进程 scheduler。
- 不实现跨机器分布式共识。
- 不保存完整 transcript 或 chain-of-thought。
- 不让 GUI、SQLite、Skill、Worker 或 GSD Adapter 成为第二个可写事实源。
- 不在 Direct 路径安装隐式 Hook 或常驻监控。
