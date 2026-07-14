---
status: accepted
---
# 三入口 Deep Kernel

CLI、GUI、MCP、Worker Runner 和 Skill Adapter 只能通过 `Apply`、`Query`、`Execute` 三个 Engine 入口工作。状态机、ownership、Evidence、Gate、Handoff、transaction 和 evolution 全部隐藏在该 Module 后面，避免每个调用端形成自己的完成语义。
