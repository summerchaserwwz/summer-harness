# Summer Harness

Summer Harness 是一套显式启用、Direct-first、local-first 的 Coding Agent 控制面。普通工作完全绕过它；顺序跨 Session 使用一个 Handoff；多阶段或并发项目使用 GSD `.planning/`；Summer 负责能力路由、一致性保护、可信交付和按需 GUI。

当前是 **v0.1 开发预览 / v3 architecture freeze**。M1 continuity 与 v1→v2 migration 已实现；Native v2 进入兼容期；machine Evidence 有 M2 在途实现；v3 Lite/GSD Router、Trust Gate、Multi-Agent、GUI 和 Evolution 尚未交付。

## 目标架构

| 路径 | 适用场景 | Authority |
|---|---|---|
| Direct | 默认问答、研究、审查、常规开发 | 无 Summer 状态 |
| Handoff Lite | 单工作流、顺序跨 Session | `.agent/HANDOFF.md` |
| Governed GSD | Phase/Wave/DAG、多 Agent、多个活跃 Session | `.planning/` |

`Direct + Skill` 只是能力叠加。多 Agent 是 GSD 硬触发；Summer 不再为新工作建立 Native Root Objective/WorkItem Workflow。

## 核心原则

- **Explicit activation**：复杂度只能建议；用户明确启用后才运行 Lifecycle Router。
- **One handoff**：`.agent/HANDOFF.md` 是唯一公开恢复入口，≤4 KiB、`must_read`≤5。
- **One owner per fact**：Git、Lite Handoff、GSD `.planning`、Trust Journal、Evidence Store、Projection 各自只拥有自己的事实。
- **Stage capability routing**：每个 activity/stage 最多 1 primary + 2 supporting Skills。
- **Single Coordinator**：Worker 只交 Proposal；Authority transition 串行提交。
- **Trust before done**：Evidence、Execution、Review、GateReceipt 绑定当前 WorkRef/workflow/tree/evidence-set；只有合格 CompletionAuthorization 能推进 terminal transition。
- **Optional product shell**：只有 `summer ui` 才加载 GUI、SQLite、Watcher 和 Graph。

## 当前可用能力

| 能力 | 状态 |
|---|---|
| Native v2 `summer start/save/resume/doctor` | 已实现，legacy compatibility |
| transaction digest chain、CAS、幂等、single writer | 已实现 |
| Handoff/Snapshot rebuild、v1→v2 dry-run/migration/rollback | 已实现 |
| `Engine.Execute` / machine Evidence | M2 在途，未发布 |
| Handoff Lite Go writer / v3 migration | 计划中 |
| Governed GSD / Capability Router / Coordinator | 计划中 |
| GUI / Evolution / Host Adapter / Release | 计划中 |

目标命令和 schema 记录在 v3 规格中，不代表当前二进制已支持。

## 当前开发预览

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
```

现有 Native v2 项目可以继续已授权的在途工作：

```bash
summer resume
summer doctor
summer save --done "<verified result>" --next "<one action>" --validation "<evidence>"
```

`summer save` 只用于已经授权、且尚未进入 migration fence 的既有 Native 在途工作。不要为新项目选择 Native v2。v3 migration 尚未实现前，需要顺序交接使用 `$project-handoff`；需要重型 Workflow 直接使用 GSD，并让 `.planning/` 成为唯一 Workflow authority。

## Native v1/v2 兼容

当前 `summer migrate` 只实现 v1→v2：

```bash
summer migrate --dry-run
summer migrate
summer resume
summer doctor
```

v2→v3 的目标契约要求：零写 dry-run、原字节备份、semantic equivalence、CAS Handoff switch、persistent tombstone、首个 v3 write 前 rollback。它将在后续 Milestone 实现，不能用手工改 Handoff/Ledger 代替。

## 设计边界

- GSD 拥有重型 Workflow；Summer 不复制它的 Phase/Plan/Wave。
- Matt Skills 提供窄工程能力；`ask-matt` 不作为第二 Router。
- Missions 只借鉴 Claim Coverage、proof scope、limited validation、production wiring 和 independent review；CSV 不是 Authority。
- Harness Anything 只借鉴 provenance、immutable records、Gate 和 rebuildable projection；Summer 不复制其重型控制面。
- 不使用 Superpowers、Super Dev、旧 Coding Agent Harness 或 Stellarlink 作为默认流程。

## 开发验证

```bash
go test ./internal/...
go test -race ./...
go vet ./...
python3 -m unittest tests.test_harnessctl -q
python3 scripts/system_doctor.py
```

## 设计资料

- [v3 产品规格](docs/product-spec-v3.md)
- [v3 系统架构](docs/architecture-v3.md)
- [v3 数据模型](docs/data-model-v3.md)
- [领域语言](CONTEXT.md)
- [交付 Roadmap](docs/roadmap.md)
- [威胁模型](docs/threat-model.md)
- [可交互架构图](docs/diagrams/summer-harness-v3.html)
- [Native v2 历史架构](docs/architecture-v2.md)

License: [Apache-2.0](LICENSE)
