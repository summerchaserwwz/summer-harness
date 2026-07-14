## 全局界面默认

- 未指定视觉方向时参考 VoltAgent DESIGN.md：近黑画布、细边框、紧凑开发者工具密度、克制的 emerald、Inter/system 字体、少装饰、无功能性 emoji。
- 工具/桌面应用优先最小实用界面；设置放到齿轮或后台，不做营销首页。

## 全局语言默认

- 默认用中文回答，除非用户明确要求其他语言。
- 用户可见 UI、状态、错误和说明默认中文；代码标识符、API/协议和第三方名称可保留英文。

## 默认工作方式

- Direct-first：问答、研究、审查、单点修复和常规开发直接完成；不初始化 Harness、不生成流程文档、不为形式拆阶段。
- 仅当用户明确说“使用 Summer Harness”“走 Harness”或调用 `$summer-harness` 时启用完整 Harness；复杂度只能作为建议理由。
- “保存交接”“下个 session 继续”“恢复工作”只触发 `$project-handoff`，不等于启用 Harness。
- 研究/解释/审查默认只读；开发、修复、构建或明确交付才授权改动。意图不明时先只读或最小澄清。
- 改码后做风险相称的真实验证；无法验证时说明。优先既有模式和最小必要改动。

## 轻量路由

每个请求只有一个生命周期所有者，再按需叠加窄 Skill：

1. `Direct`：默认路径，零持久状态。
2. `Direct + Skill`：直接完成，只加载一个必要能力。
3. `Direct + Handoff`：仅保存 `.agent/HANDOFF.md`。
4. `Summer native`：已显式授权 Harness，且需持久状态、证据或门禁。
5. `GSD backend`：已显式授权 Harness/GSD，且确需多阶段 fresh-context；`.planning/` 是唯一主账本。

Matt 只作能力插件：逐问需求用 `grilling`，根因用 `diagnosing-bugs`，代码边界用 `codebase-design`，领域模型用 `domain-modeling`；TDD/review 仅在明确要求或风险门需要时调用。`grilling` 一次问一个决策，达成共识前暂停实现。`ask-matt` 不作为第二路由器。

gstack 不自动路由；仅在用户点名具体 Skill 时调用。其 session、telemetry、Issue、commit/checkpoint 不属于 Summer Canonical 状态，也不扩大授权。产品、设计和 QA 默认仍走 Direct。

不默认或隐式使用 Super Dev、Superpowers、旧 Coding Agent Harness、Stellarlink。任何 Skill 都不得建立并列状态源。

## 唯一交接与恢复

- 唯一入口是 `.agent/HANDOFF.md`，上限 4 KiB/五个 `must_read`；完整语义留在唯一主账本。Handoff 只含当前工作集和指针，不保存聊天、思维链或源码副本。
- Summer native：`.agent/ledger/` 权威，Handoff 派生。GSD：`.planning/` 权威，Handoff 只含指针/恢复命令。Direct：简短快照。
- 新 session 先读适用 `AGENTS.md` 和 `git status`；仅在非 idle Handoff 存在或用户要求恢复时读取它。不要默认扫描 `.planning/`、旧 Harness、全量历史或聊天。
- 上下文过长、跨阶段、切换 session 或多代理并发前，更新目标、完成项、唯一下一步、验证、阻塞和 must-read。
- 仅 Summer native 可晋升有来源的 Decision/append-only Fact；旧 Fact 用 invalidate 失效。恢复只加载有界胶囊。
- GSD pause/resume 仅是 backend 内部状态；公开保存/恢复先经过 `$project-handoff` 和 Handoff。
- 优先 `summer resume`/`summer doctor`；v2 可重建缺失 Handoff，但漂移、冲突和不安全引用必须 fail-closed，禁止从聊天猜测或静默覆盖。

## 控制与安全

- 不覆盖 `.env`，不 `git push --force`；push 前检查 remote。删除外部盘、大范围文件、应用包或不确定路径前确认。
- 只改相关且已理解的文件，保留用户改动；未授权不发布、不发外部消息、不建 PR。
- 多代理仅在用户要求或明显可并行时使用；并发改码用独立 worktree/branch，主代理唯一写状态账本。
- 最新、官方、外部或高风险事实查一手来源；“完成”必须有验证，不能只信 Agent 声明。
