---
status: superseded
superseded_by: architecture-v3.md
---

# Summer Harness v2 Architecture

> 历史实现契约：用于 Native v2 兼容读取与迁移审计。当前目标架构见 [architecture-v3.md](architecture-v3.md)。本文件中的 Root Objective、WorkItem 和全域 Canonical Ledger 不再是新工作的目标模型。

## 结论

v2 采用 `headless deep kernel + on-demand product shell`：Go 单二进制承载 CLI、Kernel 和嵌入式 Web UI；GUI、MCP、GSD 与宿主 Agent 都是 Adapter。Summer 不复制宿主的进程调度器；Direct 请求不进入 Harness，`summer ui` 才加载 Web Server、Watcher、SQLite 和关系图。

## Deep Kernel Interface

外部只有三个入口：

```go
type Engine interface {
    Apply(ctx context.Context, command Command) (Receipt, error)
    Query(ctx context.Context, query Query) (View, error)
    Execute(ctx context.Context, spec RunSpec) (ExecutionReceipt, error)
}
```

这是最终产品 Interface。M1-A 只暴露已经有真实实现与测试的 `Apply / Query`；`Execute` 在 M2 的 `RunSpec`、process capture 和 `ExecutionReceipt` 完成后加入，避免用空类型制造虚假能力。

- `Apply`：所有权威状态变化、CAS、Gate、Handoff 和事务。
- `Query`：Resume、Attention、Task、Graph、Health 和 Capabilities。
- `Execute`：真实运行命令并生成绑定当前代码树和修订的 Evidence。

CLI、GUI HTTP、MCP 和 Skill Adapter 只能通过这三个入口工作。删除 Engine 后，一致性、Gate、ownership、Evidence、Handoff 和 evolution 复杂性会散回所有调用端，因此 Engine 是真正的深 Module。

## Modules

| Module | 隐藏的实现 | Interface 使用者 |
|---|---|---|
| Kernel | Command、状态机、revision、Gate、transaction | CLI、GUI、MCP、Agent Adapter |
| Ledger | transaction chain、atomic commit、fsync、recovery、upcast | Kernel |
| Continuity | Handoff、capsule、Attention、compaction | Kernel |
| Evidence | process capture、redaction、digest、artifact retention | Kernel |
| Collaboration | Assignment、lease、Proposal、ingest、worktree provenance | Kernel、Agent Adapter |
| Evolution | pattern、Candidate、Policy patch、verify、rollback | Kernel |
| Projection | snapshot、SQLite、FTS、typed graph | Query、GUI |
| Backend | Summer Native 与 GSD Pointer 的互斥 lifecycle Adapter | Kernel、GUI |

## Canonical Ledger

目标布局：

```text
.agent/
├── HANDOFF.md
├── project.json
├── ledger/
│   ├── HEAD
│   ├── transactions/
│   │   └── tx_<ULID>/
│   │       ├── manifest.json
│   │       └── 0001.json, 0002.json, ...
│   └── objects/sha256/<prefix>/<digest>
├── inbox/                  # Worker Proposal；ingest 后保留或归档
├── cache/                  # 可删除 snapshot；默认 ignored
└── runtime/                # owner lock、pending marker、heartbeat、socket；全部 ignored
```

Canonical Ledger 是 append-only committed transaction chain。每个 transaction 绑定前驱 digest、事件 digest、Actor、Session、correlation 和 idempotency key。`HEAD` 只指向最后一个完成 fsync 的 transaction。Projection、实体 Markdown 摘要和 Handoff 均由 Ledger 派生。

Handoff 是有界 Projection，不复制完整 Canonical 状态。v2 Handoff 保存当前目标、Ledger cursor、Objective identity、有限 Attention 字段，以及完整 Acceptance 的 count/digest；`summer resume` 从 Ledger 恢复完整但不超过 32 KiB 的 Capsule。提交前必须证明 Handoff 与 Capsule 都能生成，旧 v1/GSD Handoff 必须显式迁移。Handoff 缺失可从 Ledger 重建，内容漂移或冲突则 fail-closed。

一个 Ledger Store 实例只承载一个 Project。Memory 与 File Adapter 共享同一单项目、CAS、幂等和 ID 唯一性契约；第二个 Project 必须使用另一个 Store 实例。

Git 提供额外历史、同步与 Review，但运行时正确性不依赖 Git commit 是否及时发生。

## State Model

```text
Root Objective
  ├── WorkItem A -> Assignment -> Proposal -> Execution -> Review
  ├── WorkItem B -> Assignment -> Proposal -> Execution -> Review
  └── WorkItem C -> unclaimed
```

Root Objective 是唯一项目生命周期。Worker 不能直接更新 Root Objective 或 Handoff。Coordinator ingest Proposal 后，Kernel 校验 owner、capability、base SHA、allowed paths、Evidence 和 expected revision，再提交权威 transaction。

## Command Model

所有命令至少包含：

```text
command_id, idempotency_key, correlation_id, expected_revision,
project_id, actor_id, session_id, runtime, role, issued_at, payload
```

核心命令族：

```text
StartObjective, UpdateObjective, CreateWorkItem,
AssignWork, ReleaseWork, IngestProposal,
RecordDecision, RecordFact, InvalidateFact,
AttachEvidence, SubmitExecution, SubmitReview,
AddRelation, ProposeEvolution, ApproveEvolution,
ApplyPolicy, RollbackPolicy, CompleteObjective
```

拒绝原因必须机器可读，例如 `REVISION_CONFLICT`、`NOT_OWNER`、`SCOPE_VIOLATION`、`EVIDENCE_MISSING`、`GATE_FAILED` 和 `PROJECTION_DRIFT`。

## Immutable Records

Execution 必须绑定当前 WorkItem revision、base/head commit、tree digest、deliverables、Evidence refs、known gaps 和 residual risks。

Review 必须绑定 Execution、task revision、tree digest 和 evidence-set digest。任一绑定内容变化都会使 Review 失效。高风险 Review 还要求 Reviewer 不是 Execution contributor，且 Session 不同。

## GUI Read Path

```text
Canonical Ledger
      |
      +--> Snapshot projector --> CLI fast read
      |
      +--> SQLite projector --> Attention / Search / Graph / GUI
      |
      +--> Handoff projector --> next Session
```

`summer ui` 校验 projection cursor 与 Ledger HEAD。漂移时增量刷新，损坏时删除重建。GUI 写操作调用同一进程内 Engine 或 loopback JSON interface，不能直接更新 SQLite。

## GSD

项目只有两种互斥模式：

- Summer owner：`.agent/ledger/` canonical；不得调用会写 `.planning/` 的 GSD workflow。外部计划只能作为无状态输入导入，不能维护并列生命周期。
- GSD owner：`.planning/` canonical；Summer GUI 通过只读 Adapter 投影，`.agent/HANDOFF.md` 只保存路径、digest 和恢复命令。

## Failure Recovery

- command idempotency 防止重试重复提交。
- expected revision 防止丢失更新。
- transaction directory + HEAD digest chain 防止半提交。
- orphan 仅在存在唯一合法后继、且与本机 ignored pending marker 精确匹配时自动 adopt；没有 marker 的外来 orphan 与分叉都 fail-closed。
- Projection 可以删除重建。
- Handoff 缺失可从 Ledger 重建；若完整 transaction chain 证明 canonical revision 前进，则过期 Handoff 可从新状态重建。同 revision 不同 digest、倒退或不可证明的内容漂移必须 fail-closed，不能反向覆盖 Ledger。
- Snapshot 命中前仍验证完整 transaction chain，并处理本机 pending marker；Snapshot 只避免重复 fold，不替代 Ledger 完整性检查。
- v1 迁移先 `doctor`、dry-run、备份、导入、重放对比，再原子切换。

## Security Boundary

- GUI 只绑定 `127.0.0.1` 随机端口，并使用启动时生成的短期 token。
- Evidence 默认 argv 执行，不隐式经过 shell。
- stdout/stderr 执行 secret redaction、大小限制和 digest 保存。
- Worker capability 防止误提交和串单，但不宣称能抵抗已完全控制本机的恶意进程。
- Plugin 只返回 Proposal、Evidence draft 或 Gate result；Kernel 是唯一写入者。

## Implementation Sequence

1. 冻结 v1 golden tests、schema 和性能基线。
2. 建立 Go Engine/Ledger，兼容读取 v1，Python CLI 暂作 shim。
3. 实现 transaction ledger、v1 importer、fault-injection 和 migration dry-run。
4. 实现 machine Evidence、Execution、Review 和 revision-bound Gate。
5. 实现 Root Objective、WorkItem、Assignment、Proposal 和 Coordinator ingest。
6. 实现 snapshot/SQLite projection 与 read-only GUI。
7. 让 GUI 写入全部穿过 Engine，完成 Attention、Graph、Evidence 和 Agent 页面。
8. 实现 Evolution Inbox、approve/apply/verify/rollback。
9. 实现 Codex/Claude/GSD Host Adapter：导出 Assignment capsule、接收 Proposal、校验 runtime/worktree provenance；并发、预算、取消和进程生命周期继续由宿主负责。
10. 完成 Wails 桌面 Adapter、签名、公证、跨平台与公开发布。
