## 语言与界面

- 用中文回答；UI 和错误默认中文，标识符与第三方名称可保留英文。

## 默认工作方式

- Direct-first：问答、研究、审查、单点修复和常规开发直接完成；不初始化 Harness、不为形式拆阶段。
- 只有用户明确说“使用 Summer Harness”“走 Harness”或调用 `$summer-harness` 才启用 Summer；复杂度只能产生建议。
- “保存交接”“下个 Session 继续”“恢复工作”只触发 `$project-handoff`，不等于启用完整 Harness。
- 研究、解释和审查默认只读；明确要求开发、修复、构建或交付才授权改动。改动后做风险相称的真实验证。

## 三路径路由

一个请求只有一个生命周期；Skill 只是按需叠加的能力：

1. `Direct`：默认，零 Summer 状态；可临时使用一个必要的窄 Skill。
2. `Handoff Lite`：一个顺序工作流跨 Session；`.agent/HANDOFF.md` 保存当前工作集。
3. `Governed GSD`：Phase/Wave/DAG、多 Agent 或多个活跃 Session；`.planning/` 是唯一 Workflow authority。

多 Agent、多个活跃 Session、Phase/Wave/DAG 是 GSD 硬触发。Lite 只能显式晋升 GSD；两者不可双写，GSD 不可静默降级。

当前 v0.1 的 Native v2 仅作 legacy compatibility。新工作禁用；既有 Native 只允许恢复、doctor、显式迁移或已授权的兼容写入。未实现的 v3 命令不得宣称可用。

## Capability Router

- Lifecycle Router 在 Summer 启用后只选 Lite/GSD；Capability Router 再为当前 activity/stage 选择 Skill。
- 默认一个 primary、最多两个 supporting Skills；需要更多能力时拆 Assignment。
- Matt 只提供窄能力；`ask-matt` 不作第二 Router。gstack 只按用户点名使用；其他旧 Harness 不默认启用。
- Skill 不拥有生命周期，不直接写 Handoff、`.planning` 或 Trust state。

## 交接与恢复

- `.agent/HANDOFF.md` 是唯一公开恢复入口：≤4 KiB、`must_read`≤5；Resume Capsule≤32 KiB。
- Lite 只允许顺序 Writer；GSD Handoff 只保存 `.planning` 指针、digest、当前 Phase/Plan 和恢复命令。
- 新 Session 先读 `AGENTS.md` 和 `git status`；仅在需要恢复时读取 Handoff，不扫描全部 `.planning`、历史或聊天。
- 漂移、冲突、不安全路径和 stale Gate 必须 fail-closed，禁止根据聊天、mtime 或 Agent 自信猜测恢复。

## 多 Agent 与完成

- 只有 Coordinator 可以推进 `.planning`、Handoff 和 Trust acceptance；lease/epoch 放在 Git common-dir 的共享路径。
- Worker 使用独立 worktree/branch，只拿有界 Assignment，只交 immutable Proposal；不得修改 `.agent/HANDOFF.md`、`.planning/**` 或 Trust state。
- Coordinator 接收 Proposal 时重验 diff、paths、SHA、dependency 和 Evidence freshness。代码可并行，权威写入必须串行。
- machine-required Gate 不能由手写“测试通过”满足。Evidence 必须与 Claim 范围匹配，并绑定当前 WorkRef、workflow 和 tree；任一变化使旧结果 stale。
- Failed Gate 永不授权；Limited 需要 Policy 允许和可信 Host 的明确用户交互。只有 CompletionAuthorization 能允许一次精确终态转换。
- 高风险 Review 不得由同一 contributor/session 自审。Evolution、Policy、Skill、AGENTS 或代码规则不得由 Agent 自动批准修改。

## 安全边界

- 不覆盖 `.env`，不 `git push --force`；push 前核对 remote。删除外部盘、大范围文件、应用包或不确定路径前确认。
- 只改相关且已理解的文件，保留用户改动；未授权不发布、不发外部消息、不建 PR。
- GUI、SQLite、Graph、Skill、Worker、Plugin 和 Adapter 都不能成为第二 Authority。
- 最新、官方、外部或高风险事实查一手来源；“完成”必须有与声明范围匹配的验证。
