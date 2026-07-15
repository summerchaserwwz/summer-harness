# Summer Harness Architecture

本文是稳定导航入口。当前权威目标架构是 [architecture-v3.md](architecture-v3.md)。

Native v2 的 transaction ledger、Root Objective 和 WorkItem 模型已进入兼容期；历史契约保留在 [architecture-v2.md](architecture-v2.md)，用于读取、审计和显式迁移，不能作为新工作流的设计依据。

## 一句话架构

```text
Direct（默认，零 Summer 状态）
  |
  +-- 顺序跨 Session --> Handoff Lite (.agent/HANDOFF.md)
  |
  +-- Phase/Wave/DAG、多 Agent、多活跃 Session
                          |
                          v
                  Governed GSD (.planning/)
                          |
                          v
         Capability Router + Coordinator Guard
                          |
                          v
 Evidence -> Execution -> Review -> GateReceipt -> CompletionAuthorization
                          |
                          v
              Handoff / Resume / on-demand GUI
```

Summer 不再为新工作创建 Native Phase/WorkItem Workflow。它把重型 Workflow 交给 GSD，把自身复杂度集中在统一恢复、阶段能力路由、本地并发一致性和可信交付。

## 权威边界

- Git：代码、commit、tree。
- `.agent/HANDOFF.md`：Lite 当前工作集；GSD 模式下只是 pointer/digest。
- `.planning/`：GSD Requirement、Phase、Plan、Wave、Task。
- Summer Trust Journal：immutable Evidence、Execution、Review、GateReceipt 和 provenance。
- private Evidence Store：redacted raw logs 与 Artifact。
- SQLite/FTS/Graph/GUI：可删除重建的 Projection。

一类事实只能有一个正式 Writer。

## 设计索引

- [v3 系统架构](architecture-v3.md)
- [v3 产品规格](product-spec-v3.md)
- [v3 数据模型](data-model-v3.md)
- [领域语言](../CONTEXT.md)
- [威胁模型](threat-model.md)
- [交付 Roadmap](roadmap.md)
- [ADR](adr/)
- [可交互架构图](diagrams/summer-harness-v3.html)
- [Native v2 历史架构](architecture-v2.md)
