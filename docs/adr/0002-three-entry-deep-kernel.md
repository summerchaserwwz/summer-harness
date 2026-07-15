---
status: accepted
---
# 三入口 Deep Kernel

CLI、GUI、MCP、Host Agent Adapter 和 Skill Adapter 只能通过 `Apply`、`Query`、`Execute` 三个 Engine 入口工作。Evidence、Gate、Handoff validation、transaction、coordination 和 evolution 隐藏在该 Module 后面，避免每个调用端形成自己的完成语义。Summer 不复制宿主的进程调度器。

v3 补充：Engine 是深接口和 Trust/validation 边界，不是第二 Workflow owner。Lite working set 由 Handoff 拥有；重型 Workflow 由 GSD `.planning/` 拥有。Engine 可以接受已验证的 Workflow checkpoint，但不得镜像 GSD Task status。
