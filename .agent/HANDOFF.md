---
{
  "blockers": [],
  "done": [
    "M0 已完成架构收敛、公开仓库与 Direct-first/唯一 Handoff/单账本设计冻结。",
    "M1-A 已完成 Go Apply/Query 与安全 File/Memory Ledger foundation，独立架构、安全和 UX 审计问题已收敛。"
  ],
  "engine": "summer",
  "goal": "把 Summer Harness 迭代为易安装、易使用、默认轻量、支持可靠跨 session、多 Agent 协作、候选式自我进化和真实 GUI 看板的开源 Coding Agent Harness，并发布到 summerchaserwwz/summer-harness。",
  "last_writer": "session_59bd844fa73bdd89",
  "mode": "native",
  "must_read": [
    "docs/product-spec-v2.md",
    "docs/architecture-v2.md",
    "docs/data-model-v2.md",
    "docs/threat-model.md",
    "docs/roadmap.md"
  ],
  "next": [
    "实现 Go Handoff projector vertical slice，并产出首条真实可用的 summer resume 命令。"
  ],
  "resume_command": "$project-handoff",
  "schema": "summer-harness/v1",
  "source_digest": "f45224bd35d8479fa9fb6747ba283ac8375f39275ea4496d72ea2346e0fe68f6",
  "source_path": ".agent/ledger/tasks/task_20260714T154324166611Z_8f7b2c.md",
  "task_id": "task_20260714T154324166611Z_8f7b2c",
  "task_status": "active",
  "updated_at": "2026-07-14T18:09:08.652347Z",
  "validation": [
    "harnessctl doctor 返回 ok=true 且无 warnings。",
    "20 项 Python v1 回归测试通过。",
    "system_doctor 与 harnessctl doctor 均通过。",
    "Archify JSON/HTML 校验通过，公开路径与 secret 扫描通过，git diff --check 通过。",
    "Go test、race 与 vet 全部通过；35 个 Go 测试函数覆盖跨进程锁、CAS、幂等、恢复、secret 与输入边界。",
    "关键并发/幂等/orphan/跨进程用例连续 20 次通过。",
    "Python v1 回归 22 项通过，system_doctor 与 harnessctl doctor 通过。",
    "三个 Skill quick_validate、gofmt、git diff --check、公开路径与高置信 secret 扫描通过。"
  ]
}
---
# Project Handoff

## 当前目标

- 把 Summer Harness 迭代为易安装、易使用、默认轻量、支持可靠跨 session、多 Agent 协作、候选式自我进化和真实 GUI 看板的开源 Coding Agent Harness，并发布到 summerchaserwwz/summer-harness。

## 已完成

- M0 已完成架构收敛、公开仓库与 Direct-first/唯一 Handoff/单账本设计冻结。
- M1-A 已完成 Go Apply/Query 与安全 File/Memory Ledger foundation，独立架构、安全和 UX 审计问题已收敛。

## 唯一下一步

- 实现 Go Handoff projector vertical slice，并产出首条真实可用的 summer resume 命令。

## 验证

- harnessctl doctor 返回 ok=true 且无 warnings。
- 20 项 Python v1 回归测试通过。
- system_doctor 与 harnessctl doctor 均通过。
- Archify JSON/HTML 校验通过，公开路径与 secret 扫描通过，git diff --check 通过。
- Go test、race 与 vet 全部通过；35 个 Go 测试函数覆盖跨进程锁、CAS、幂等、恢复、secret 与输入边界。
- 关键并发/幂等/orphan/跨进程用例连续 20 次通过。
- Python v1 回归 22 项通过，system_doctor 与 harnessctl doctor 通过。
- 三个 Skill quick_validate、gofmt、git diff --check、公开路径与高置信 secret 扫描通过。

## 必须读取

- docs/product-spec-v2.md
- docs/architecture-v2.md
- docs/data-model-v2.md
- docs/threat-model.md
- docs/roadmap.md
