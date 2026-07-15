## 全局界面默认

- 未指定视觉方向时参考 VoltAgent DESIGN.md：近黑画布、细边框、紧凑开发者工具密度、克制的 emerald、Inter/system 字体、少装饰、无功能性 emoji。
- 工具/桌面应用优先最小实用界面；设置放到齿轮或后台，不做营销首页。

## 全局语言默认

- 默认用中文回答，除非用户明确要求其他语言。
- 用户可见 UI、状态、错误和说明默认中文；代码标识符、API/协议和第三方名称可保留英文。

## 默认工作方式

- Direct-first：问答、研究、审查、单点修复和常规开发直接完成；不初始化 Harness、不生成流程文档、不为形式拆阶段。
- 仅当用户明确说“使用 Summer Harness”“走 Harness”或调用 `$summer-harness` 时启用 Summer；复杂度只能作为建议理由。
- “保存交接”“下个 session 继续”“恢复工作”只触发 `$project-handoff`，不等于启用完整 Harness。
- 研究/解释/审查默认只读；开发、修复、构建或明确交付才授权改动。改码后做风险相称的真实验证。

## 三路径路由

每个请求只有一个生命周期，能力 Skill 只是按需叠加：

1. `Direct`：默认路径，零 Summer 状态。可加载一个必要的窄 Skill，但这不是第四个生命周期。
2. `Handoff Lite`：只解决单工作流、顺序跨 Session；`.agent/HANDOFF.md` 是当前工作集。
3. `Governed GSD`：Phase/Wave/DAG、多 Agent 或多个活跃 Session；`.planning/` 是唯一 Workflow authority。

多 Agent、多个活跃 Session、Phase/Wave/DAG 是 GSD 硬触发。Lite 只能显式单向晋升 GSD；GSD 不可静默降级或与 Lite 双开。

当前 v0.1 的 Native v2 `start/save/resume/doctor` 是 legacy compatibility。不要为新工作选择 Native；现有 Native 项目只能继续已授权在途工作、诊断或等待显式 v3 migration，不得把目标 v3 命令宣称为当前已实现。
既有在途工作在 migration fence 安装前可受限使用当前 `summer save`；fence 存在后所有 legacy write 必须 fail-closed。

## Capability Router

- Lifecycle Router 只选 Lite/GSD；Capability Router 每个 activity/stage 选择 SkillPlan。
- 默认一个 primary、最多两个 supporting Skills；需要更多能力时拆 Assignment。
- Lite 把当前紧凑 SkillPlan 保存在 Handoff；GSD 每个 routed activity 保存 immutable ActivitySkillPlan，Worker Assignment 引用同一 Plan digest；Trust 只引用 Plan digest。
- Matt 只作窄能力：`grilling`、`domain-modeling`、`codebase-design`、`diagnosing-bugs`、`tdd`、`code-review`。`ask-matt` 不作为第二 Router。
- gstack 仅在用户点名具体 Skill 时使用；其 session、telemetry、Issue、commit/checkpoint 不属于 Summer Authority。
- 不默认或隐式使用 Super Dev、Superpowers、旧 Coding Agent Harness、Stellarlink。
- Skill 不拥有生命周期，不直接写 Handoff、`.planning` 或 Trust Journal。

## 唯一交接与恢复

- `.agent/HANDOFF.md` 是唯一公开恢复入口，上限 4 KiB、五个 `must_read`；Resume Capsule 上限 32 KiB。
- Lite：Handoff 保存当前工作集，仅允许顺序 Writer 和 revision CAS。
- GSD：`.planning/` 拥有 Workflow；Handoff 只保存 pointer、snapshot digest、当前 Phase/Plan 和恢复命令。
- Legacy Native：只读恢复、doctor 或显式迁移；新 write 应返回 `MIGRATION_REQUIRED`（目标行为，尚未实现前不得伪装）。
- 新 Session 先读适用 `AGENTS.md` 和 `git status`；仅在非 idle Handoff 存在或用户要求恢复时读取它，不默认扫描 `.planning`、全量历史或聊天。
- 漂移、冲突、不安全引用和 stale Gate 必须 fail-closed，禁止从聊天、mtime 或 Agent 猜测恢复。

## 多 Agent 一致性

- 只有 Coordinator 可以推进 `.planning`、Handoff 和 Trust acceptance。
- 项目 lease/epoch 必须位于 `git rev-parse --git-common-dir` 可共享路径。
- Worker 使用独立 worktree/branch，只拿 Assignment Capsule，只交 immutable Proposal。
- Worker 禁止直接修改 `.agent/HANDOFF.md`、`.planning/**` 或 Trust Journal；Coordinator ingest 时重新计算 Git diff、paths、SHA、dependency 和 Evidence freshness。
- 并发写码可以并行，Authority 写入必须串行。首发不宣称跨机器分布式一致性。

## Trust 与完成

- machine-required Gate 不能由手写“测试通过”或 manual attestation 满足。
- Evidence 同时表达 capture trust 与 proof scope；低等级 Evidence 不能支持更高范围 Claim。
- Execution/Review/GateReceipt 绑定 WorkRef、workflow/source digest、tree digest 和 evidence-set digest；任一变化使旧结果 stale。
- GateReceipt 只是 evaluation，并绑定 SkillPlan/Gate set/Policy digest。Failed 永不授权；Limited 需要 Policy 允许和可信 Host 的明确用户交互，ActorRef 不能代替授权；只有 CompletionAuthorization 能允许一次 exact terminal transition。
- Reviewer 高风险自审禁止；Evolution approval/second confirmation 必须来自可信 Host 的明确用户交互，不能靠 ActorRef 或 Agent 自报。

## 控制与安全

- 不覆盖 `.env`，不 `git push --force`；push 前检查 remote。删除外部盘、大范围文件、应用包或不确定路径前确认。
- 只改相关且已理解的文件，保留用户改动；未授权不发布、不发外部消息、不建 PR。
- GUI、SQLite、Graph、Skill、Worker、Plugin 和 Adapter 都不能成为第二 Authority。
- 最新、官方、外部或高风险事实查一手来源；“完成”必须有与声明范围相匹配的验证。
