# Summer Harness v2 Architecture

## 结论

v2 采用 `headless deep kernel + on-demand product shell`：Go 单二进制承载 CLI、Kernel 和嵌入式 Web UI；GUI、MCP、Worker Runner、GSD 与宿主 Agent 都是 Adapter。Direct 请求不进入 Harness，`summer ui` 才加载 Web Server、Watcher、SQLite 和关系图。

## Deep Kernel Interface

外部只有三个入口：

```go
type Engine interface {
    Apply(ctx context.Context, command Command) (Receipt, error)
    Query(ctx context.Context, query Query) (View, error)
    Execute(ctx context.Context, spec RunSpec) (ExecutionReceipt, error)
}
```

- `Apply`：所有权威状态变化、CAS、Gate、Handoff 和事务。
- `Query`：Resume、Attention、Task、Graph、Health 和 Capabilities。
- `Execute`：真实运行命令并生成绑定当前代码树和修订的 Evidence。

CLI、GUI HTTP、MCP 和 Skill Adapter 只能通过这三个入口工作。删除 Engine 后，一致性、Gate、ownership、Evidence、Handoff 和 evolution 复杂性会散回所有调用端，因此 Engine 是真正的深 Module。

## Modules

| Module | 隐藏的实现 | Interface 使用者 |
|---|---|---|
| Kernel | Command、状态机、revision、Gate、transaction | CLI、GUI、MCP、Runner |
| Ledger | transaction chain、atomic commit、fsync、recovery、upcast | Kernel |
| Continuity | Handoff、capsule、Attention、compaction | Kernel |
| Evidence | process capture、redaction、digest、artifact retention | Kernel |
| Collaboration | Assignment、lease、Proposal、ingest、worktree provenance | Kernel、Runner |
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
│   │       └── events/*.json
│   └── objects/sha256/<prefix>/<digest>
├── inbox/                  # Worker Proposal；ingest 后保留或归档
├── cache/                  # 可删除 snapshot；默认 ignored
└── runtime/                # lock、heartbeat、socket、临时 transaction
```

Canonical Ledger 是 append-only committed transaction chain。每个 transaction 绑定前驱 digest、事件 digest、Actor、Session、correlation 和 idempotency key。`HEAD` 只指向最后一个完成 fsync 的 transaction。Projection、实体 Markdown 摘要和 Handoff 均由 Ledger 派生。

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

- Summer owner：`.agent/ledger/` canonical；GSD 只能提供计划或 Worker 能力，不能维护并列生命周期。
- GSD owner：`.planning/` canonical；Summer GUI 通过只读 Adapter 投影，`.agent/HANDOFF.md` 只保存路径、digest 和恢复命令。

## Failure Recovery

- command idempotency 防止重试重复提交。
- expected revision 防止丢失更新。
- transaction directory + HEAD digest chain 防止半提交。
- orphan 仅在存在唯一合法后继时自动 adopt；分叉时 fail-closed。
- Projection 可以删除重建。
- Handoff 漂移从 Ledger 重建，不能反向覆盖 Ledger。
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
9. 实现 Codex/Claude Runner、队列、预算、取消、重试和 worktree lifecycle。
10. 完成 Wails 桌面 Adapter、签名、公证、跨平台与公开发布。
