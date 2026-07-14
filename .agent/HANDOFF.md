---
{
  "blockers": [],
  "done": [
    "M0/M1-A 已完成架构冻结、公开仓库和 Go transaction Ledger foundation。",
    "M1-B 已完成 Continuity vertical slice：真实 summer resume/doctor、v1 三模式恢复、v2 有界 Handoff/Capsule、缺失投影重建、生命周期/容量 fail-closed 与 CLI 边界防护。"
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
    "实现 M1-C：v1 到 v2 的 dry-run/import/rollback 迁移、可验证 snapshot fast path，以及 native summer start/save 写命令。"
  ],
  "resume_command": "$project-handoff",
  "schema": "summer-harness/v1",
  "source_digest": "dae7eccee251be5fc2103aea31f93df1601ec349e143798e95202d24340f5d86",
  "source_path": ".agent/ledger/tasks/task_20260714T154324166611Z_8f7b2c.md",
  "task_id": "task_20260714T154324166611Z_8f7b2c",
  "task_status": "active",
  "updated_at": "2026-07-14T19:40:14.005674Z",
  "validation": [
    "Go test、race 与 vet 全部通过；35 个 Go 测试函数覆盖跨进程锁、CAS、幂等、恢复、secret 与输入边界。",
    "关键并发/幂等/orphan/跨进程用例连续 20 次通过。",
    "Python v1 回归 22 项通过，system_doctor 与 harnessctl doctor 通过。",
    "三个 Skill quick_validate、gofmt、git diff --check、公开路径与高置信 secret 扫描通过。",
    "Go 全量 test、race、vet 通过；新增 P1 故障测试覆盖超限状态、双生命周期、缺失投影、幂等重试与 symlink cwd。",
    "Python v1 23 项回归、三个 Skill quick_validate、system_doctor 和 git diff --check 通过。",
    "真实 ~/.local/bin/summer --version/resume/doctor 对当前项目通过。",
    "Resume p95 <100ms 性能测试连续运行 20 轮通过。"
  ]
}
---
# Project Handoff

## 当前目标

- 把 Summer Harness 迭代为易安装、易使用、默认轻量、支持可靠跨 session、多 Agent 协作、候选式自我进化和真实 GUI 看板的开源 Coding Agent Harness，并发布到 summerchaserwwz/summer-harness。

## 已完成

- M0/M1-A 已完成架构冻结、公开仓库和 Go transaction Ledger foundation。
- M1-B 已完成 Continuity vertical slice：真实 summer resume/doctor、v1 三模式恢复、v2 有界 Handoff/Capsule、缺失投影重建、生命周期/容量 fail-closed 与 CLI 边界防护。

## 唯一下一步

- 实现 M1-C：v1 到 v2 的 dry-run/import/rollback 迁移、可验证 snapshot fast path，以及 native summer start/save 写命令。

## 验证

- Go test、race 与 vet 全部通过；35 个 Go 测试函数覆盖跨进程锁、CAS、幂等、恢复、secret 与输入边界。
- 关键并发/幂等/orphan/跨进程用例连续 20 次通过。
- Python v1 回归 22 项通过，system_doctor 与 harnessctl doctor 通过。
- 三个 Skill quick_validate、gofmt、git diff --check、公开路径与高置信 secret 扫描通过。
- Go 全量 test、race、vet 通过；新增 P1 故障测试覆盖超限状态、双生命周期、缺失投影、幂等重试与 symlink cwd。
- Python v1 23 项回归、三个 Skill quick_validate、system_doctor 和 git diff --check 通过。
- 真实 ~/.local/bin/summer --version/resume/doctor 对当前项目通过。
- Resume p95 <100ms 性能测试连续运行 20 轮通过。

## 必须读取

- docs/product-spec-v2.md
- docs/architecture-v2.md
- docs/data-model-v2.md
- docs/threat-model.md
- docs/roadmap.md
