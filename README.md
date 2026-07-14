# Summer Harness

Summer Harness 是一套显式启用、Git-native、local-first 的 Coding Agent continuity kernel。普通任务完全绕过它；启用后，它用一个 Canonical Ledger 和一个 Handoff 解决跨 Session 恢复，再按风险增加 Evidence、Review、多 Agent 治理与可控进化。

> 当前是开发 checkpoint，不是 v0.1 安装发行版。Go CLI 已提供真实的 `summer resume`、`summer doctor` 和 `summer --version`；写命令暂时仍由 Python v1 shim 承担。Homebrew、签名 Release 和 `summer setup` 仍在路线图中。

默认情况下 Agent 直接完成任务，不创建 Harness 状态。只有用户明确说“使用 Summer Harness / 走 Harness”时，才在项目中创建 `.agent/`；跨 session 只读取 `.agent/HANDOFF.md`，不会重放整段对话。

## 设计边界

- Direct-first：普通任务不进入 Harness。
- One handoff：`.agent/HANDOFF.md` 是唯一恢复入口。
- One source of truth：原生模式以 `.agent/ledger/` 为主账本；GSD 模式以 `.planning/` 为主账本，Handoff 只保存指针。
- Semantic memory：长期保存目标、Decision、Fact、Evidence、已完成、下一步、阻塞与风险，不默认保存聊天流水账或思维链。
- Risk-shaped gates：标准任务只要求验收条件和真实验证；高风险/发布任务再增加独立审查。
- Disposable sessions：新 session 读取有限的恢复胶囊，而不是全量历史。

## 从源码安装当前 CLI

需要 Go 1.26 或更高版本：

```bash
go install ./cmd/summer
summer --version
summer resume
summer doctor
```

如果 `$GOPATH/bin` 不在 `PATH`，可以直接构建到已有的本地命令目录：

```bash
go build -o ~/.local/bin/summer ./cmd/summer
```

`summer resume` 和 `summer doctor` 可以在项目子目录运行，也可显式指定仓库：

```bash
summer --repo /path/to/project resume
summer doctor --repo /path/to/project --json
```

## 当前写入流程

M1 阶段写操作仍使用仓库内的 Python v1 shim；恢复与健康检查使用 Go CLI：

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
summer resume
summer doctor
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

维护者检查本机全局 Skill/GSD/gstack 安装（普通用户不需要运行）：

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

当前开发状态：M1-B 已建立 Go `Apply / Query` vertical slice、单 Project Memory/File Ledger、transaction digest chain、CAS、幂等、跨进程 Writer 锁、崩溃恢复、v1 兼容读取、v2 Handoff projector，以及真实的 `summer resume/doctor`。Handoff 缺失时可由 Canonical Ledger 重建；漂移、旧生命周期冲突和不可投影状态会 fail-closed。

设计资料：

- [v2 产品规格](docs/product-spec-v2.md)
- [v2 系统架构](docs/architecture-v2.md)
- [领域语言](CONTEXT.md)
- [交付路线图](docs/roadmap.md)
- [可交互架构图](docs/diagrams/summer-harness-v2.html)
