# Summer Harness Delivery Roadmap

这不是缩减范围的 MVP。每个 milestone 都是最终目标的一部分；排序用于控制风险和保证前一层能够验证后一层。

## M0 — Architecture Freeze and Baseline

- 冻结 v1 行为、schema、故障注入测试和性能基线。
- 建立领域语言、产品规格、ADR 和 v2 架构图。
- 首次 Git commit，创建公开仓库前完成 secret/license 审查。

## M1 — Go Deep Kernel

- Go module、`Apply / Query` Engine vertical slice、File/Memory Ledger Adapter；`Execute` 在 M2 具备真实 Evidence contract 后加入。
- v1 reader、兼容 CLI、golden contract tests。
- transaction chain、idempotency、CAS、fsync、recovery、snapshot。
- `summer migrate --dry-run` 与可回滚迁移。

## M2 — Trusted Delivery

- `summer run -- <argv>` machine Evidence。
- immutable Execution / Review。
- Evidence trust level、secret redaction、artifact digest 和 retention。
- revision/tree/evidence-set bound completion gates。

## M3 — Multi-Agent Governance

- Root Objective + WorkItem。
- Assignment、ownership、lease、allowed paths、worktree provenance。
- Worker Proposal、Coordinator ingest、scope 和 base SHA 校验。
- 中断后可恢复的 Agent/WorkItem 状态。

## M4 — Product GUI

- React/Vite 共享 UI，Go embed，`summer ui` loopback server。
- Attention/Resume、Work、Graph、Evidence、Agent、Evolution、Health、Settings。
- SQLite/FTS/typed graph projection、增量刷新和完整 rebuild。
- VoltAgent 风格 token、响应式布局、键盘导航和可访问性。

## M5 — Controlled Evolution

- Candidate discovery、source refs、counterexample、risk 和 expected benefit。
- GUI diff approval、apply、verify、rollback。
- project/user scope Policy；全局变更二次确认。
- Summer Harness 使用自身 Candidate 流程改进自己的 Policy 和 Skill。

## M6 — Built-in Worker Runner

- Codex/Claude CLI Adapter。
- 队列、并发、预算、超时、取消、重试。
- worktree/branch 创建、恢复、清理和 merge gate。
- Worker 日志与 Evidence 进入受控存储，不直接写 Canonical Ledger。

## M7 — Installation and Desktop

- GitHub Release、checksums、SBOM、签名和 Homebrew tap。
- `summer setup codex|claude` 幂等安装，不覆盖用户配置。
- Wails native shell，共用 Engine 与 React UI。
- macOS 公证；Windows/Linux CI 与安装渠道。

## M8 — Open Source Release

- Apache-2.0、中文/英文 README、quickstart、architecture、security、contributing。
- 示例项目、录屏、截图、迁移指南和故障恢复手册。
- GitHub Actions 覆盖 unit、contract、fault injection、GUI、cross-platform、release smoke。
- 创建并推送 `summerchaserwwz/summer-harness`。
- 形成公众号文章素材：问题、取舍、架构图、性能和 dogfood 案例。

## Release Gates

- 所有 milestone 验收均有 machine Evidence。
- 独立 Review 绑定当前代码树、Task revision 和 Evidence 集合。
- `summer doctor`、迁移、projection rebuild、Handoff 恢复和安装 smoke 全绿。
- Direct 与 Resume 性能达到产品规格。
- GUI 数据全部能追溯到 Canonical Ledger，删除 Projection 后重建一致。
- 公开仓库不存在 secret、私有路径、个人 token 或不可再分发资产。
