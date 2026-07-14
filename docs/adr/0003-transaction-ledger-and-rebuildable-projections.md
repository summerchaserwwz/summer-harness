---
status: accepted
---
# Transaction Ledger 与可重建 Projection

v2 以 Git 可追踪的 append-only committed transaction chain 作为 Canonical Ledger，Handoff、实体摘要、snapshot、SQLite 和关系图都是 Projection。相比继续扩展可变 Task 文件或让 SQLite 成为真相，该模型能为多 Agent CAS、immutable Execution/Review、崩溃恢复和审计提供统一基础，同时让 Direct 路径完全绕过投影。
