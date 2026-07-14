## 全局界面默认

- 用户未指定视觉方向时，以 VoltAgent DESIGN.md 为灵感：近黑画布、细边框、紧凑开发者工具密度、克制的 emerald 强调色、Inter/system 字体、少装饰、无功能性 emoji。
- 工具与桌面应用优先采用最小实用界面；设置放到齿轮或独立后台，不做营销式首页。

## 全局语言默认

- 默认用中文回答，除非用户明确要求其他语言。
- 用户可见的 UI 文案、状态、错误、设置和说明默认使用中文；代码标识符、API 字段、协议名和第三方产品名可保留英文。

## 默认工作方式

- Direct-first：普通问答、研究、审查、单点修复和常规开发直接完成，不初始化 Harness，不生成流程文档，不为了形式拆阶段。
- 只有用户明确说“使用 Summer Harness”“走 Harness”或显式调用 `$summer-harness`，才启用完整 Harness。任务复杂只能成为建议理由，不能自动启用。
- 用户只说“保存交接”“下个 session 继续”“恢复工作”时，使用 `$project-handoff`；这不等于启用 Harness。
- 分析、研究、解释、审查和比较默认只读；开发、修复、构建和明确交付才授权改动。意图不明时先只读检查或做最小澄清。
- 代码改动后必须做与风险相称的真实验证；无法验证时明确说明。优先简单方案、既有模式和最小必要改动。

## 轻量路由

每个请求只选择一个生命周期所有者，再按需叠加窄能力 Skill：

1. `Direct`：默认路径，零持久状态。
2. `Direct + Skill`：任务本身仍直接完成，只加载一个确有必要的能力 Skill。
3. `Direct + Handoff`：只为跨 session 保存 `.agent/HANDOFF.md`。
4. `Summer native`：用户显式要求 Harness，且任务边界明确但需要持久 Task / Decision / Fact、证据或风险门禁。
5. `GSD backend`：用户显式要求 Harness 或 GSD，且工作确实是多阶段、需要 fresh-context 规划执行；`.planning/` 是唯一主账本。

Matt Skills 只作为能力插件：bug 根因用 `diagnosing-bugs`，代码边界用 `codebase-design`，领域建模用 `domain-modeling`；TDD 和 review 仅在用户明确要求或风险门需要时调用。`ask-matt` 仅用于人工帮助/导航，不自动调用，不作为第二个路由器。

gstack 同样只作为窄能力：需求澄清用 `spec` 或 `plan-ceo-review`，设计用 `design-consultation` / `design-review`，浏览器与 QA 用 `browse` / `qa` / `qa-only`，落地前审查用 `review`。不因任务属于产品、设计或 QA 就自动进入完整流程。

不使用 Super Dev、Superpowers、旧 Coding Agent Harness 或 Stellarlink Harness 作为默认或隐式工作流。GSD、Matt、gstack 或其他 Skill 都不能建立与当前生命周期所有者并列的状态源。

## 唯一交接与恢复

- 项目跨 session 的唯一入口是 `.agent/HANDOFF.md`，限制在 4 KiB 内、最多五个 `must_read` 文件；不保存聊天流水账、思维链或大段源码。
- Summer native 模式以 `.agent/ledger/` 为权威状态，Handoff 由 Task 派生；GSD 模式以 `.planning/` 为权威状态，Handoff 只保存指针和恢复命令；Direct 模式只保存简短快照。
- 新 session 进入项目时先读适用的 `AGENTS.md` 和 `git status`。仅当存在非 idle 的 `.agent/HANDOFF.md`，或用户要求继续/恢复时，才读取 Handoff 并按其模式恢复；不要默认扫描 `.planning/`、旧 Harness、所有历史或聊天记录。
- 上下文过长、跨阶段、准备切换 session 或多代理并发前，先更新 Handoff：当前目标、已完成、唯一下一步、验证、阻塞和必须读取文件。
- 关键长期信息只晋升为有来源的 Decision 或 append-only Fact；旧 Fact 通过 invalidate 失效，不无痕改写。恢复时只加载有界胶囊，不加载全量账本。

## 控制与安全

- 不覆盖 `.env`，不使用 `git push --force`；push 前先检查 `git remote -v`。删除外部盘、大范围文件、应用包或不确定路径前先确认。
- 只修改相关且已理解的文件；保留用户已有改动。没有明确授权时不发布、不发送外部消息、不创建 PR。
- 多代理仅在用户要求或能明显并行的复杂任务中使用；并发写代码必须用独立 worktree/branch，主代理是状态账本的唯一写入者。
- 需要最新、官方、外部或高风险事实时查一手来源。任何“完成”都必须由验证结果支持，不能只凭 Agent 声明。
