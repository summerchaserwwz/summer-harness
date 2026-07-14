# Summer Harness Architecture

本文是稳定导航入口。当前权威架构是 [architecture-v2.md](architecture-v2.md)；早期 Task + Handoff 两文件 WAL 属于 v1 legacy，不再描述当前写入模型。

## 一句话架构

```text
Direct（默认，零状态）
  |
  +-- 只需跨 Session --> .agent/HANDOFF.md
  |
  +-- 显式 Summer -----> Go Engine -> .agent/ledger transaction chain
  |                                      +-> bounded Handoff/Snapshot
  |                                      +-> optional SQLite/GUI projection
  |
  +-- 显式 GSD --------> .planning/ canonical -> pointer Handoff
```

Native v2 的 Canonical Ledger 是 append-only committed transaction chain。Handoff、Snapshot、未来 SQLite、Graph 和 GUI 都是可验证或可重建的 Projection；Git 提供额外历史，但不承担运行时原子性。

## 轻量边界

- 普通请求不初始化 Harness，不扫描仓库，不启动 daemon。
- `AGENTS.md` 是唯一常驻路由；Adaptive Router 只保留作旧配置兼容。
- Matt Skills 是能力插件，不是第二生命周期。
- gstack 只在用户显式点名具体 Skill 时调用，其状态不进入 Summer Ledger。
- GSD 只有成为 lifecycle owner 时才能写 `.planning/`；Summer Native owner 不调用 GSD 状态型流程。
- `summer ui` 才按需加载 GUI、Watcher、SQLite 和关系图，关闭后不影响 CLI 与恢复。

## 设计索引

- 完整模块、Ledger、Failure Recovery、GUI read path：[architecture-v2.md](architecture-v2.md)
- 产品目标、性能预算、GUI 与多 Agent 体验：[product-spec-v2.md](product-spec-v2.md)
- 事件与领域模型：[data-model-v2.md](data-model-v2.md)
- 安全与信任边界：[threat-model.md](threat-model.md)
- 关键架构决策：[adr/](adr/)
- 交付顺序与 Release Gate：[roadmap.md](roadmap.md)
