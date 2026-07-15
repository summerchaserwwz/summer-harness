---
status: accepted
supersedes:
  - product-spec-v2.md
---

# Summer Harness v3 产品规格

## 产品定义

Summer Harness 是显式启用、local-first 的 Coding Agent 控制面：

- 普通工作保持 Direct；
- 轻量顺序续接使用一个 Handoff；
- 重型与并发项目交给 Governed GSD；
- Summer 在两个持久模式之上提供 Capability Router、可信交付、一致性保护和按需 GUI。

它不是模型、Prompt 大礼包、通用 Issue Tracker、Worker scheduler 或第二套 GSD。

## 目标结果

1. 新 Session 在有界输入内恢复唯一下一步。
2. 多 Agent 不争写 Workflow、Handoff 或 Trust records。
3. 每个阶段按需求选择最小必要 Skill，而不是加载整套流程。
4. “完成”由真实 Evidence、独立 Review、绑定当前 Gate contract 的 GateReceipt，以及被 exact checkpoint 消费的 CompletionAuthorization 共同证明。
5. GUI 能解释当前状态，但删除 GUI 数据不影响恢复。
6. 重复失败只形成待批准 Evolution Candidate，不自动修改规则。

## 三种体验

### Direct

```text
用户提出普通请求
  -> Host 直接工作
  -> 可按需调用一个窄 Skill
  -> Summer 不创建文件、不扫描仓库、不启动进程
```

### Handoff Lite

适用：一个目标、顺序执行、只需跨 Session。

目标命令表面：

```bash
summer start "<goal>"              # explicit activation + explainable auto route
summer start --lite "<goal>"
summer save --done "<result>" --next "<one action>" --validation "<evidence>"
summer resume
summer doctor
```

Lite 不支持并行 Writer。第二个活跃 Writer、Phase/Wave/DAG 或用户要求多 Agent 时，返回 GSD route reason 并执行显式 promotion。

Promotion 使用 common-dir fence、expected Lite digest、GSD target preimage、staging/pending marker、target CAS、Handoff CAS 和 committed tombstone。崩溃恢复必须证明唯一 successor；不能通过同时保留可写 Lite 与 `.planning/` 来“提高成功率”。

### Governed GSD

适用：多阶段、长期 Roadmap、依赖 Wave、多 Agent、多个活跃 Session。

目标命令表面：

```bash
summer start --gsd "<goal>"
summer route --explain
summer promote gsd
summer resume
summer doctor
```

`.planning/` 是 Workflow authority；Summer Handoff 只提供 pointer/digest 和恢复命令。Summer 的 Evidence/Review/Gate 不复制 GSD Task status。

## 当前 v0.1 过渡表面

当前二进制仍实现 Native v2 Objective `start/save/resume/doctor` 和 v1→v2 migration。这是 legacy compatibility，不是 v3 目标能力。

在 v3 migration 命令交付前：

- 不为新项目推荐 Native v2；
- 现有 Native v2 项目可以只读恢复、运行 doctor；迁移交付前，只有已经授权且未命中 migration fence 的在途工作可以继续使用当前 `summer save` compatibility writer；
- 不把目标 `--lite`、`--gsd`、Capability Router、GUI 或 Gate 命令宣称为已实现；
- 需要重型 Workflow 时直接使用 GSD，并让 `.planning/` 成为唯一 Workflow authority。

## Public Surface

### 基础命令

| 命令 | 目标行为 |
|---|---|
| `summer` | 只读 Attention；未启用时零写入 |
| `summer route --explain` | 解释建议，不创建状态 |
| `summer start [--lite|--gsd]` | 显式授权后自动路由；flag 可覆盖 Backend |
| `summer save` | Lite 保存或 GSD checkpoint pointer |
| `summer resume` | 输出 ≤32 KiB Capsule |
| `summer promote gsd` | Lite→GSD dry-run + CAS 切换 |
| `summer run -- <argv>` | 捕获 machine Evidence |
| `summer check` | 计算 Delivery Coverage/Gate readiness |
| `summer doctor` | 诊断 Authority、Handoff、Trust、Projection、Adapter |
| `summer ui` | 按需加载产品壳 |

目标命令在实现前必须标注“planned”。

## Capability Routing

每个 activity/stage 输入 Goal、Acceptance、WorkRef、risk、artifact、failure signal、Evidence gap、allowed effects、Host 能力和 context budget。

输出 `SkillPlan`：

- 一个 primary；
- 最多两个 support；
- inline/fresh/parallel-wave strategy；
- expected Artifact；
- required Evidence/Gates；
- route reasons；
- Skill version/digest。

用户可以查看和覆盖 optional Skill；Policy-required Skill 缺失时返回 `SKILL_UNAVAILABLE` 或 `limited`，不能假装完成。

Lite 把当前 activity 的紧凑 SkillPlan 直接保存在 Handoff 并纳入 4 KiB preflight。GSD 的每个 routed activity 都保存 immutable `ActivitySkillPlan` metadata；Worker Assignment 引用同一 Plan digest，Discuss/Plan/inline integration/fresh review 不要求伪造 worktree Assignment。Trust records 只引用 Plan digest，不能形成第三份可变 Plan。

Contract Registry content-addressed 保存被使用过的 SkillManifest、SkillPlan snapshot、完整 GateSpec set 和 Gate Policy canonical bytes。Lite/GSD Plan 在首次被 Evidence/Execution/Gate 引用前归档 immutable snapshot；它只提供历史反查，不成为第二个可变 Plan Authority。所有 Receipt digest 必须可反查原文，重复 digest/WorkRef 必须满足 canonical 等式。Active Gate/permission Contract 一律使用 scope-bound trusted Host user approval，不提供 baseline shortcut；更新创建新 version，不能覆盖旧 contract。

## Multi-Agent Experience

GSD Plan/Wave 产生 Assignment Capsule。Worker 在独立 worktree/branch 中工作，只交 Proposal；Coordinator 串行接收。

用户能看到：

- 当前 Coordinator/epoch；
- Worker、WorkRef、branch/worktree、base SHA；
- allowed paths、SkillPlan 和 Evidence requirements；
- Proposal、Review、Gate readiness；
- stale、overlap、drift 和恢复动作。

Worker 不能直接更新 `.planning/`、Handoff 或 Trust Journal。首发只承诺同一 Git common-dir 下的本地一致性。

## Trusted Delivery

`summer run -- <argv>` 目标捕获：

- argv、repo-relative cwd；
- start/end/duration、exit/signal；
- Git HEAD、dirty tree digest、changed paths；
- stdout/stderr size、truncation、summary、content digest；
- tool version、value-free environment summary、Artifact digest；
- Actor/Session、WorkRef、Workflow snapshot digest。

Gate 同时检查：

- Evidence trust；
- proof scope；
- Claim coverage；
- production wiring；
- tree/workflow/evidence-set freshness；
- Reviewer independence。

结果只有 `verified`、`limited`、`failed`。Manual attestation 不能满足 machine-required Gate。

GateReceipt 是 evaluation，不直接拥有完成权限，并必须绑定 SkillPlan、required Gate set、实际 Gate Policy 和 ApprovedContractRecord digest。Contract approval 使用不含 approval 引用的 candidate，trusted receipt 绑定 candidate，独立 approval record 再组合二者，禁止摘要环；三者 scope/project/WorkIdentity 必须一致，GateReceipt 只接受 global、同 Project 或与当前 WorkRef identity 相等的 approval。`failed` 永不授权；`limited` 只有 Gate contract 明确允许，且 Host 通过模型/Coordinator 无法伪造的明确用户交互 challenge 生成 scope/project/WorkIdentity-bound TrustedUserInteractionReceipt，再由其生成 UserAcceptanceReceipt 时才生成 CompletionAuthorization；没有可信 Host user channel 就返回 `USER_ACCEPTANCE_UNAVAILABLE`。`ActorRef.Role=user` 只做 provenance，不能授权。`verified` 可按同一 Policy 生成。Adapter 先在 staging 中生成 successor，Authorization 精确复用 GateReceipt 的 Plan/Gate/Policy/approval digest，并绑定 acceptance、expected previous、authorized successor、tree/evidence-set 和 fencing epoch。成功 checkpoint 必须精确匹配并引用 Authorization digest；之后它只保留为历史依据。Cancellation 使用 purpose=`cancel` 的独立 interaction receipt 与 CancellationAuthorization，不构成完成。

## GUI

只有 `summer ui` 才启动 loopback HTTP、Watcher、SQLite/FTS/Graph。静态资源嵌入 Go binary；未来 Wails 复用同一 Engine 和前端。

页面：

1. Resume / Attention：唯一下一步、blocker、drift、stale Evidence、pending Review/Proposal。
2. Workflow：Lite working set 或 GSD Phase/Plan/Wave，只读显示 authority cursor。
3. Coverage：criterion、implementation、Evidence、Review、wiring、git/proposal state。
4. Evidence：真实命令回执、Git/Workflow 绑定和 redaction 状态。
5. Agents：Coordinator、Worker、worktree、lease、Proposal。
6. Evolution：Candidate、diff、approval、validation、rollback。
7. Graph：WorkRef、Claim、Evidence、Execution、Review、Gate、Actor relation。
8. Health / Settings：Authority、Adapter、Projection、retention、privacy。

GUI 写操作必须调用 Engine；SQLite 与浏览器前端不能直接写 `.planning`、Handoff 或 Trust Journal。

## Controlled Evolution

状态机：

```text
candidate -> approved | rejected
approved  -> applied
applied   -> verified | rolled_back
```

每个 Candidate 必须有 source refs、occurrences、counterexamples、expected benefit、scope、risk、patch、validation 和 rollback。批准和 global second confirmation 必须引用 immutable TrustedUserInteractionReceipt，purpose 分别为 `evolution-approve|evolution-confirm`，绑定同一 Candidate target digest，且两次确认使用不同 InteractionID；模型、Worker、Coordinator 或 `ActorRef.Role=user` 都不能生成。Host 无可信用户通道时 Candidate 只能保持 pending/rejected，不能 apply。

## Performance Budgets

- Direct：零常驻进程、零 Summer 文件写入、零全仓扫描。
- `summer resume`：p95 <100ms，不加载 Node、GUI、SQLite。
- Handoff：≤4 KiB；must-read≤5；Capsule≤32 KiB。
- `summer ui`：cold first-interaction <2s；warm <1s。
- 1,000 个投影实体的 Attention/Graph query <200ms。
- Projection 删除后可从其 Authority 重建相同 content digest。

## Distribution

- GitHub：`summerchaserwwz/summer-harness`，Apache-2.0。
- 首发渠道：GitHub Release、checksums、SBOM、Homebrew、`go install`。
- `summer setup codex|claude` 必须幂等，不覆盖用户配置；测试使用隔离 HOME。
- macOS 桌面签名/公证、Windows/Linux 渠道按 Release Goal 验证。
- 中文为默认文档和 UI；同时提供英文 README、architecture、security、contributing。

## Explicit Non-acceptance

- 普通请求隐式启用 Harness。
- 为新工作创建 Summer Native Objective/WorkItem Workflow。
- Handoff 与 `.planning` 双写 GSD Task 状态。
- Skill、Worker、GUI、SQLite 或 Plugin 直接写 Authority。
- Summer 复制 Host Worker scheduler。
- CSV 成为 Canonical state。
- Agent 审批自己的高风险 Review。
- 未经 User 批准自动修改 Policy、Skill、AGENTS 或代码。
- 通过 mock、fixture、dry-run 或文字声明冒充真实集成/E2E/副作用 Evidence。
