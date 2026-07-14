# Summer Harness

Summer Harness 是一套显式启用、Git-native、local-first 的 Coding Agent continuity kernel。普通任务完全绕过它；大项目启用后，用一个 Canonical Ledger 和一个 `.agent/HANDOFF.md` 保证跨 Session 可恢复，再按里程碑增加可信 Evidence、多 Agent 治理、可控经验进化和按需 GUI。

当前是 **v0.1 开发预览**。Continuity 与 v1→v2 迁移基座已可运行；Evidence、Multi-Agent、GUI 与 Evolution 仍在开发，不会伪装成已经交付的命令。

## 核心原则

- **Direct-first**：不显式调用 Summer 时，零常驻进程、零扫描、零写入。
- **One handoff**：`.agent/HANDOFF.md` 是唯一公开的跨 Session 入口。
- **One source of truth**：Native 使用 `.agent/ledger/`；GSD 使用 `.planning/`，两者绝不并列。
- **Bounded restore**：Handoff 不超过 4 KiB，恢复 Capsule 不超过 32 KiB，最多五个 `must_read`。
- **Fail closed**：同 revision 摘要冲突、生命周期冲突、不安全路径、伪造 Snapshot 或不完整迁移都拒绝继续；只有可由更高 canonical revision 证明的过期投影会自动重建。
- **Optional product shell**：未来只有 `summer ui` 才加载 GUI、SQLite、Watcher 与关系图，不影响 Direct 和 CLI 快路径。

## 当前可用能力

| 能力 | 状态 |
|---|---|
| `summer start/save/resume/doctor` | 已实现 |
| Transaction digest chain、CAS、幂等、跨进程单 Writer | 已实现 |
| 缺失 Handoff/Snapshot 的 Canonical 重建 | 已实现 |
| v1 全历史 dry-run、导入、双读验证、rollback journal | 已实现 |
| Decision / Fact / machine Evidence / Review Gate | 计划中（M2） |
| WorkItem / Assignment / Proposal / 多 Agent 恢复 | 计划中（M3） |
| Attention、Graph、Evidence、Agent、Evolution GUI | 计划中（M4） |
| Evolution Inbox 与 Host Agent Adapter | 计划中（M5/M6） |

## 安装开发预览

需要 Go 1.26 或更高版本：

```bash
go install github.com/summerchaserwwz/summer-harness/cmd/summer@latest
summer --version
```

从源码构建：

```bash
git clone https://github.com/summerchaserwwz/summer-harness.git
cd summer-harness
go build -o ~/.local/bin/summer ./cmd/summer
summer --version
```

GitHub Release、Homebrew、checksums、签名和 `summer setup codex|claude` 属于发行里程碑，当前尚未提供。

## 30 秒使用

只有明确执行 `start` 才创建 Harness 状态：

```bash
cd /path/to/project

summer start "交付登录功能"
summer save \
  --done "完成会话模型" \
  --next "实现登录端点" \
  --validation "go test ./... 通过" \
  --must-read "docs/auth.md"

summer resume
summer doctor
```

`start` 未提供 `--next` 时，会把 Goal 作为第一个 Next；也可显式覆盖：

```bash
summer start "交付登录功能" --next "先确认会话边界"
```

已有 `$project-handoff` 生成的 Direct/Idle Handoff 时，显式 `summer start` 会把它替换为 Native v2。v1 Native 必须迁移，GSD 生命周期必须先由用户结束或切换，`start` 不会抢占它们。

命令可以在仓库子目录执行，也可指定根目录；`--json` 提供稳定机器输出：

```bash
summer --repo /path/to/project resume --json
summer doctor --repo /path/to/project
```

CLI 退出码：`0` 表示 transaction 与投影都成功；`1` 表示内部错误；`2` 表示参数错误或提交前拒绝；`3` 表示 canonical transaction 已提交，但 Handoff/Snapshot 投影需要修复。`--json` 在退出码 `3` 时返回 `ok:false`、`committed:true`、`projection:"repair_required"`、`code` 和修复提示，不能把它当作未提交重试。

## 从 V1 迁移

Native v1 必须显式迁移，不能让旧 Python writer 与 v2 并行写入：

```bash
summer migrate --dry-run
summer migrate
summer resume
summer doctor
```

迁移会验证 Handoff、Task、Decision、Fact、路径、密钥模式和容量，备份完整 v1 原始字节，并在一个 genesis transaction 中导入全部历史。只有迁移后没有任何新 v2 transaction 时才允许：

```bash
summer migrate --rollback
```

Migration/rollback 可在崩溃后重试；不要手工移动 `.agent/ledger/`、migration archive、HEAD 或 Handoff。

迁移后，原 v1 `tasks/`、`decisions/`、`facts/` 会暂留在 `.agent/ledger/` 并完整复制到 migration archive，用于 rollback 与审计；它们不再是写入源。只要 `.agent/ledger/HEAD` 存在，唯一 canonical 状态就是 `HEAD + transactions/`，不要继续使用 Python v1 writer 或手改遗留文件。

## 工作流组合

- 普通问答、研究、审查和常规开发：Direct。
- 只需跨 Session：`$project-handoff`，不启用完整 Harness。
- 显式要求 Summer：Native v2，`.agent/ledger/` canonical。
- 显式要求 GSD 且确实多阶段：GSD backend，`.planning/` canonical，Handoff 只保存指针。
- Matt Skills：只作 `grilling`、诊断、代码边界、领域建模等能力插件。
- gstack：仅用户显式点名具体 Skill；其 session、Issue、commit 或 telemetry 不属于 Summer 状态。

不使用 Superpowers、Super Dev、旧 Coding Agent Harness 或 Stellarlink Harness 作为默认或隐式工作流。Compatibility Adaptive Router 不进入默认安装表面。

## 为什么不是 Harness Anything 的复制品

Summer 借鉴了 Harness Anything 的 Canonical state、Provenance、Evidence、Review 与可重建 Projection 思想，但把日常路径压缩为 Direct、Handoff 或一个显式生命周期所有者。GUI 是按需产品壳，不是常驻控制面；SQLite 和关系图永远可删除重建；首要目标是跨 Session 连续性，而不是为每个小改动生成完整治理文档。

## 开发验证

```bash
go test ./internal/...
go test -race ./...
go vet ./...
python3 -m unittest tests.test_harnessctl -q
```

## 设计资料

- [产品规格](docs/product-spec-v2.md)
- [v2 系统架构](docs/architecture-v2.md)
- [领域语言](CONTEXT.md)
- [交付路线图](docs/roadmap.md)
- [威胁模型](docs/threat-model.md)
- [可交互架构图](docs/diagrams/summer-harness-v2.html)

License: [Apache-2.0](LICENSE)
