# Summer Harness

Summer Harness 是一套显式启用、文件型、零依赖的个人 Coding Agent 工作流。当前仓库同时包含已经可用的 v1 Kernel，以及正在通过自身 Harness 账本推进的 v2 产品化设计。

> 当前是开发 checkpoint，不是 v0.1 安装发行版。下面的 Python v1 命令需要在本仓库内运行；稳定的全局 `summer` binary、Homebrew 安装和公开 setup 流程仍在路线图中。

默认情况下 Agent 直接完成任务，不创建 Harness 状态。只有用户明确说“使用 Summer Harness / 走 Harness”时，才在项目中创建 `.agent/`；跨 session 只读取 `.agent/HANDOFF.md`，不会重放整段对话。

## 设计边界

- Direct-first：普通任务不进入 Harness。
- One handoff：`.agent/HANDOFF.md` 是唯一恢复入口。
- One source of truth：原生模式以 `.agent/ledger/` 为主账本；GSD 模式以 `.planning/` 为主账本，Handoff 只保存指针。
- Typed memory：长期信息只晋升为 Task、Decision、Fact，不保存聊天流水账。
- Risk-shaped gates：标准任务只要求验收条件和真实验证；高风险/发布任务再增加独立审查。
- Disposable sessions：新 session 读取有限的恢复胶囊，而不是全量历史。

## 快速使用

```bash
python3 skills/summer-harness/scripts/harnessctl.py init
python3 skills/summer-harness/scripts/harnessctl.py start \
  --title "任务标题" \
  --goal "要达到的结果" \
  --acceptance "可验证的验收条件"
python3 skills/summer-harness/scripts/harnessctl.py checkpoint \
  --done "已经完成的工作" \
  --next "唯一下一步" \
  --validation "已执行的验证"
python3 skills/summer-harness/scripts/harnessctl.py resume
```

完整命令见 `python3 skills/summer-harness/scripts/harnessctl.py --help`。

## 当前全局安装

- `~/.codex/AGENTS.md` 链接到 `config/AGENTS.md`。
- Summer Harness 不安装隐式 Harness/GSD session hooks，也不覆盖用户已有 hooks。仓库中的 `config/hooks.json` 只是空白安全示例；作者本机的其他 hooks 保存在被忽略的 `config/hooks.local.json`。
- `summer-harness` 与 `project-handoff` 以符号链接安装，仓库是唯一可编辑来源。
- GSD 使用 `standard` surface；Matt 仅按需安装 `grilling`、`diagnosing-bugs`、`codebase-design`、`domain-modeling`、`tdd`。
- 上游 `ask-matt` 不作为默认入口：它会建立一条完整 idea-to-ship 流程，与 Direct-first 和唯一生命周期所有者重复；需要逐问澄清时直接显式调用 `grilling`。
- GSD surface 的 Codex 路径适配已修正为 `CODEX_HOME`，并补齐本地 installer runtime，按需切换 profile 不依赖日常 session hook。
- 旧 CAH、Stellarlink Harness、Super Dev 和未选中的 gstack 入口已移到 `skills-disabled`，没有删除。

检查全局配置：

```bash
python3 scripts/system_doctor.py
```

架构与使用口令见 [docs/architecture.md](docs/architecture.md)。

## v2 产品化方向

v2 的目标不是复制一个更重的 Harness，而是把当前可靠的 Handoff 和文件事务深化为一个公开可安装的 Agent continuity kernel：

- Go 单二进制与按需启动的 React 本地 GUI。
- Root Objective + WorkItem 的多 Agent ownership。
- machine-captured Evidence、immutable Execution / Review。
- SQLite、搜索和关系图仅作为可删除重建的 Projection。
- 人工批准的 Evolution Inbox，自我进化不会自动污染规则。
- 后续内建 Codex / Claude Worker Runner，但普通任务仍完全绕过 Harness。

当前开发状态：M1-A 已建立 Go `Apply / Query` vertical slice、单 Project Memory/File Ledger、transaction digest chain、CAS、幂等、跨进程 Writer 锁和崩溃恢复。日常使用仍走上面的 Python v1 CLI；在第一条真实 `summer` 命令、Handoff projector 和兼容读取完成前，不会把 Skill 切到未完成的 Go 入口。

设计资料：

- [v2 产品规格](docs/product-spec-v2.md)
- [v2 系统架构](docs/architecture-v2.md)
- [领域语言](CONTEXT.md)
- [交付路线图](docs/roadmap.md)
- [可交互架构图](docs/diagrams/summer-harness-v2.html)
