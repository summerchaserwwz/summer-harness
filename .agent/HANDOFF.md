---
{
  "blockers": [],
  "done": [
    "已用 Summer native 建立自举任务并记录用户优先级、GUI 范围和候选式自我进化决策。",
    "已确认当前 v1 为 47KB 单文件 Python Kernel，具备 Handoff、Task、Decision、Fact、事务与完成门禁，但缺少真实 Evidence、Execution、Worker ownership、Evolution 和 GUI。",
    "三套独立 Kernel 设计已收敛，并冻结为 v2 产品规格、领域语言、数据模型、架构、威胁模型、ADR 和完整路线图。",
    "已生成并验证可交互架构图，确定 Go 单 binary、按需 React Web GUI、Wails 后续桌面壳、三入口 Engine、transaction ledger 和可重建 Projection。",
    "已将个人 Vibe Island hooks 从公开配置分离，公开仓库不会覆盖用户 hooks，本机功能仍由 ignored hooks.local.json 保留。"
  ],
  "engine": "summer",
  "goal": "把 Summer Harness 迭代为易安装、易使用、默认轻量、支持可靠跨 session、多 Agent 协作、候选式自我进化和真实 GUI 看板的开源 Coding Agent Harness，并发布到 summerchaserwwz/summer-harness。",
  "last_writer": "019f5f82-aa54-7d93-b921-181c740d3c76",
  "mode": "native",
  "must_read": [
    "docs/product-spec-v2.md",
    "docs/architecture-v2.md",
    "docs/data-model-v2.md",
    "docs/threat-model.md",
    "docs/roadmap.md"
  ],
  "next": [
    "等待用户确认三入口 Engine seam 与推荐技术选择，然后按 TDD 开始 Go Kernel vertical slice。"
  ],
  "resume_command": "$project-handoff",
  "schema": "summer-harness/v1",
  "source_digest": "d6c406a89f1a01eefb8ee00bb26d0fdce871ef9f899b3c39c414a0d83794d666",
  "source_path": ".agent/ledger/tasks/task_20260714T154324166611Z_8f7b2c.md",
  "task_id": "task_20260714T154324166611Z_8f7b2c",
  "task_status": "active",
  "updated_at": "2026-07-14T16:04:28.256005Z",
  "validation": [
    "harnessctl doctor 返回 ok=true 且无 warnings。",
    "20 项 Python v1 回归测试通过。",
    "system_doctor 与 harnessctl doctor 均通过。",
    "Archify JSON/HTML 校验通过，公开路径与 secret 扫描通过，git diff --check 通过。"
  ]
}
---
# Project Handoff

## 当前目标

- 把 Summer Harness 迭代为易安装、易使用、默认轻量、支持可靠跨 session、多 Agent 协作、候选式自我进化和真实 GUI 看板的开源 Coding Agent Harness，并发布到 summerchaserwwz/summer-harness。

## 已完成

- 已用 Summer native 建立自举任务并记录用户优先级、GUI 范围和候选式自我进化决策。
- 已确认当前 v1 为 47KB 单文件 Python Kernel，具备 Handoff、Task、Decision、Fact、事务与完成门禁，但缺少真实 Evidence、Execution、Worker ownership、Evolution 和 GUI。
- 三套独立 Kernel 设计已收敛，并冻结为 v2 产品规格、领域语言、数据模型、架构、威胁模型、ADR 和完整路线图。
- 已生成并验证可交互架构图，确定 Go 单 binary、按需 React Web GUI、Wails 后续桌面壳、三入口 Engine、transaction ledger 和可重建 Projection。
- 已将个人 Vibe Island hooks 从公开配置分离，公开仓库不会覆盖用户 hooks，本机功能仍由 ignored hooks.local.json 保留。

## 唯一下一步

- 等待用户确认三入口 Engine seam 与推荐技术选择，然后按 TDD 开始 Go Kernel vertical slice。

## 验证

- harnessctl doctor 返回 ok=true 且无 warnings。
- 20 项 Python v1 回归测试通过。
- system_doctor 与 harnessctl doctor 均通过。
- Archify JSON/HTML 校验通过，公开路径与 secret 扫描通过，git diff --check 通过。

## 必须读取

- docs/product-spec-v2.md
- docs/architecture-v2.md
- docs/data-model-v2.md
- docs/threat-model.md
- docs/roadmap.md
