# Summer Harness v2 产品规格

状态：提议中，按照推荐默认值继续实现；用户可在后续迭代中修改尚未发布的选择。

## 产品定义

Summer Harness 是一个 Git-native、local-first 的 Coding Agent continuity kernel。普通任务完全绕过它；用户显式启用后，它负责跨 Session 恢复、多 Agent ownership、真实 Evidence、审查门禁、经验候选和按需 GUI。

它不是模型，不是 Prompt 大礼包，也不要求常驻 daemon。Codex、Claude、GSD 和未来 Worker Runner 都是 Adapter；一个项目始终只有一个 lifecycle owner 和一个 Canonical Ledger。

## 目标用户

1. 经常在多个大项目和多个 Session 之间切换的个人开发者。
2. 同时使用多个 Coding Agent、模型或 worktree 的高级用户。
3. 希望获得证据链、Decision 历史、可恢复交接和可控自我进化的开源项目维护者。

## 优先级

1. 跨 Session 不丢失可行动上下文。
2. 多 Agent 协作不串单、不争写、不产生双账本。
3. 从真实失败和 Review 中生成可审查的改进候选。
4. GUI 让复杂项目状态可理解、可恢复、可操作。
5. 一键安装、低学习成本和公开可维护性。

## 默认体验

```bash
brew install summerchaserwwz/tap/summer
summer setup codex

cd my-project
summer start "交付登录功能"
summer save
summer resume
summer ui
```

未显式调用 `summer` 或 `$summer-harness` 时，不创建文件、不扫描仓库、不启动进程。

## 常用 Interface

普通用户只需要理解：

```text
summer          当前 Attention 摘要；未启用时保持只读
summer start    显式启用并创建 Root Objective
summer save     生成 checkpoint 与唯一 Handoff
summer resume   输出有界恢复胶囊
summer ui       按需启动 GUI
summer doctor   诊断账本、投影、Handoff 和安装状态
```

高级能力折叠在命名空间：

```text
summer record fact|decision
summer run -- <command>
summer work assign|claim|submit|ingest
summer review submit
summer evolve scan|approve|apply|rollback
summer agent run|cancel|retry
summer gsd ...
```

## GUI

`summer ui` 启动只绑定 loopback 的本地 Web 应用。React/Vite 静态资源嵌入 Go binary；SQLite、文件监听、HTTP 和图谱仅在该命令运行期间加载。后续以 Wails 包装同一个 Go Core 和同一套前端，提供原生桌面分发。

首页是 Resume / Attention，而不是统计卡片堆：

- 唯一主 CTA：继续当前下一步。
- 阻塞、待 Review、过期 Evidence、Handoff 漂移、未接收 Proposal、待批准 Evolution。
- Agent lane：Actor、WorkItem、worktree、branch、heartbeat、Evidence 和合并状态。
- 当前 Gate readiness、最近 Decision 和恢复命令。

其他页面：

1. Work：Root Objective 与 WorkItem DAG。
2. Graph：Task、Decision、Fact、Evidence、Execution、Review、Actor 的 typed graph。
3. Evidence：真实命令回执、摘要、Git 绑定和验证覆盖。
4. Evolution Inbox：候选来源、反例、diff、验证和 rollback。
5. Health：Ledger、Projection、Handoff、Adapter 和安装健康度。
6. Settings：生命周期后端、保留策略、Evidence 隐私和 Agent Adapter。

## 多 Agent

第一阶段先治理宿主启动的 Worker，随后实现内建 Runner。所有 Worker 必须通过 Assignment 工作，并在独立 branch/worktree 中修改代码。Worker 只能提交 Proposal；Coordinator 是 Canonical Ledger 和 Handoff 的唯一推进者。

内建 Runner 最终支持：

- Codex / Claude CLI Adapter。
- 并发上限、队列、预算、超时、取消和重试。
- worktree 自动创建、清理和恢复。
- Proposal ingest 与 merge gate。
- 失败后保留 Assignment、分支、提交和 Evidence，Session 可以丢弃。

## Evidence 与完成门禁

代码、发布和安全任务不能用手写“测试通过”满足完成门禁。`summer run -- <argv>` 必须捕获：

- argv 与 repo-relative cwd。
- 开始、结束、持续时间、退出码或 signal。
- Git HEAD、dirty tree digest、受影响文件摘要。
- stdout/stderr 大小、摘要、截断状态和内容 digest。
- 工具版本、允许公开的环境摘要和 artifact digest。
- Actor、Session、WorkItem、Task revision。

默认信任等级：

```text
observed_process > ci_attestation > file_digest > external_reference > manual_attestation
```

Manual attestation 可以解释文档或研究结果，但不能满足声明为 machine-required 的 Gate。

## 自我进化

状态机：

```text
candidate -> approved | rejected
approved  -> applied
applied   -> verified | rolled_back
```

候选来自重复 finding、重复 blocker、Evidence 失败或 Fact 模式。每个候选必须包含来源、频率、反例、预计收益、影响范围、风险、变更 diff、验证命令和 rollback。

未经 User 批准，Harness 不得修改 AGENTS.md、Skill、Policy 或代码。全局级变更需要比项目级变更更高的确认门槛。

## 状态与隐私

- Handoff、Root Objective、WorkItem、Decision、Fact、Execution manifest、Review 和 Evolution Candidate 可以进入 Git。
- stdout/stderr、截图和大型 Artifact 默认进入内容寻址 Evidence Store，并受 `.gitignore`、大小限制和 secret redaction 控制。
- Canonical record 保存 digest、大小、摘要和 retention policy；大文件可以使用外部 CI artifact URL + digest。
- SQLite、搜索索引、页面缓存、heartbeat、锁和临时事务永远不是 Canonical Ledger。

## 性能预算

- Direct：零常驻进程、零仓库扫描、零文件写入。
- `summer` / `summer resume`：p95 小于 100ms，不加载 GUI 或 SQLite。
- `summer ui`：冷启动首个可操作页面小于 2s，warm 小于 1s。
- 1,000 个实体的 Attention/Graph 查询小于 200ms。
- Handoff 不超过 4KiB；恢复胶囊不超过 32KiB。
- 删除所有 Projection 后可以从 Canonical Ledger 完整重建。

## 首发与开源

- GitHub：`summerchaserwwz/summer-harness`，Public。
- License：Apache-2.0。
- macOS 首发：GitHub Release binary、checksums、Homebrew tap、install script、`go install`。
- Windows/Linux 在核心稳定后通过 CI、Scoop/winget 和 shell installer 补齐。
- 中文文档为默认，同时提供英文 README 和贡献指南。

## 明确不接受

- GUI 直接写 Markdown 或 SQLite。
- SQLite、daemon、GSD 或 Worker 自己成为第二状态源。
- Session transcript 作为恢复主机制。
- Agent 自动批准自己的 Review。
- Evolution 未经用户确认自动改写规则。
- 插件直接取得 Ledger 文件写权限。
- 为普通任务安装隐式 Session hook。
