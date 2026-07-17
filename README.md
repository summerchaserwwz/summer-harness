# Summer Harness

[中文](README.md) | [English](README_EN.md)

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-Apache--2.0-34d399)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.1.0--dev-f59e0b)](docs/roadmap.md)

## 先说人话：这到底是什么

**Summer Harness 是给 Coding Agent 配的一套“工作纪律 + 项目接力棒 + 验收机制”。**

模型本身只是大脑。它能写代码，但聊天会变长、记忆会丢、多个 Agent 会互相覆盖，而且它很容易在没有充分验证时说“已经完成”。Harness 就是包在模型外面的工作系统，负责回答这些问题：

- 这个任务要不要进入正式流程？
- 是直接做，还是需要保存进度，还是要拆成多阶段项目？
- 当前阶段应该调用哪个 Skill，而不是一次加载所有方法论？
- 换一个 Session 后，从哪里继续，唯一下一步是什么？
- 多个 Agent 同时工作时，谁可以修改计划和最终状态？
- Agent 说“完成”时，有没有测试、Review 和真实结果支持？

如果把 Coding Agent 比作一名工程师：

| 部件 | 人话解释 |
|---|---|
| 模型 | 工程师的大脑 |
| Codex / Claude Code | 工程师的电脑、终端和双手 |
| Skill | TDD、Debug、Review 等专项工具或工作方法 |
| GSD | 复杂项目的项目经理和阶段计划 |
| Handoff | 下班或换人时留下的接力棒 |
| Summer Harness | 决定何时启用上述能力，并保护记忆、一致性和验收的工作系统 |

Summer 不会让模型突然变聪明。它的价值是让模型在长项目、多 Agent 和多 Session 中**少忘事、少冲突、少假完成**，同时不拖慢简单任务。

只记住下面五句话就够了：

1. 普通任务直接做，不启动 Harness。
2. 只想下个 Session 接着做，就保存一个 Handoff。
3. 任务需要分阶段、并行或多 Agent，就进入 GSD。
4. 每个阶段只选最需要的 Skill，不加载技能大礼包。
5. “完成”要看真实证据，不只听 Agent 自己说。

> [!IMPORTANT]
> 当前仓库是 **`v0.1.0-dev` 开发预览**。GitHub HEAD 已实现 Native v2 的连续性、冲突保护、恢复和 v1→v2 migration，但这些已经进入兼容期。上面描述的是已经固化的 v3 目标架构；自动路由、正式 Handoff Lite、Governed GSD Adapter、可信完成和 GUI 仍按 Roadmap 逐步交付。本文会明确区分“现在能用”和“目标设计”。

## 它主要解决什么问题

| 实际问题 | 你会看到的症状 | Summer 的处理方式 |
|---|---|---|
| 简单任务被重流程拖慢 | 改一行代码也先写计划、建目录、跑一套仪式 | 默认 Direct；除非你明确要求，否则 Summer 完全不启动 |
| 对话越来越长，质量下降 | Agent 忘记前面的约束，重复读文件，开始自相矛盾 | 只保存有界 Handoff；重型阶段使用 fresh context，不重放整段聊天 |
| 跨 Session 接不上 | 新窗口不知道做到哪，只能重新扫描仓库和聊天 | `.agent/HANDOFF.md` 是唯一公开恢复入口，只保留已完成、阻塞和唯一下一步 |
| Skill 太多，不知道用哪个 | `ask-matt`、TDD、Debug、Review、GSD 都在抢着接管流程 | 生命周期和 Skill 选择分开；每个阶段只选 1 个主 Skill，最多 2 个辅助 Skill |
| 多 Agent 相互覆盖 | 两个 Worker 同时改计划、状态和同一批文件 | Worker 可以并行写代码，但只有 Coordinator 能推进计划、Handoff 和验收状态 |
| Agent 过早宣布完成 | “测试应该通过”“看起来没问题”被当成完成证据 | 测试结果、代码版本、Review 和交付范围绑定；证据过期就重新验证 |
| 工具越来越重 | GUI、数据库、关系图反过来变成另一套真相 | GUI 和数据库只做可删除视图，真正状态仍由 Git、Handoff 或 GSD 持有 |

这套设计追求的不是“功能最多”，而是：**简单任务几乎零成本，复杂任务才逐步增加纪律。**

## 30 秒决定怎么用

| 你的情况 | 你需要说什么 | 实际发生什么 |
|---|---|---|
| 问问题、做研究、Review、小修复、普通开发 | 正常提出要求即可 | 走 Direct，不创建 Summer 状态 |
| 需要某个专项能力 | 可以点名 `$tdd`、`$diagnosing-bugs`、`$code-review` 等 | 仍是 Direct，只临时叠加这个 Skill |
| “今天先到这，下个 Session 继续” | 说“保存交接”或调用 `$project-handoff` | 只更新一个 `.agent/HANDOFF.md`，不会启动完整 Harness |
| 一个顺序目标，但要跨多个 Session | 明确说“使用 Summer Harness” | 目标上进入 Handoff Lite；当前过渡期使用 `$project-handoff` |
| 有 Phase、Wave、依赖图、多 Agent 或多个活跃 Session | 说“使用 GSD”或“使用 Summer Harness” | 进入 GSD；`.planning/` 是唯一项目计划来源 |
| 新窗口继续已有工作 | 说“恢复工作” | 先读 `AGENTS.md`、`git status` 和唯一 Handoff，再加载当前阶段必需内容 |

目前不要用 `summer start` 创建新的 Native v2 项目。当前 Go CLI 主要用于已有 Native 项目的兼容恢复和迁移；详见后面的“现在应该怎么使用”和“当前 CLI 表面”。

## 它怎样和 `AGENTS.md` 配合

是的，Summer 必须和项目的 `AGENTS.md` 配合，但两者不是重复关系。

### `AGENTS.md` 是常驻交通规则

Agent 每次进入仓库都会读它，所以它必须短、稳定、低成本。它负责规定：

- 默认 Direct-first；
- 只有用户明确要求才启用 Summer；
- 保存交接只调用 `$project-handoff`；
- 多 Agent 或 Phase/Wave/DAG 必须进入 GSD；
- 谁能写 `.planning/`、Handoff 和验收状态；
- 基本安全边界，例如不覆盖 `.env`、不擅自发布。

### Summer 是按需启动的运行机制

只有明确启用后，Summer 才负责选择 Lite/GSD、记录当前工作、协调多 Agent、选择阶段 Skill，以及检查交付证据。

### 为什么不把所有内容都写进 `AGENTS.md`

因为 `AGENTS.md` 每个 Session 都会进入上下文。如果把完整 GSD、所有 Skills、Evidence schema、Gate 规则和架构原理全部塞进去，简单任务还没开始就先加载几千行说明，这正是我们要消除的“重”。

正确分工是：

| 层 | 负责什么 | 是否每次加载 |
|---|---|---|
| `AGENTS.md` | 最小路由、安全和权限规则 | 是 |
| `$project-handoff` | 单工作流跨 Session 接力 | 需要交接或恢复时 |
| GSD `.planning/` | 重型项目的需求、阶段、计划和任务 | 进入当前 GSD 阶段时 |
| Capability Skills | Debug、TDD、建模、Review 等专项能力 | 当前阶段需要时 |
| Summer Trust | Evidence、Review、Gate 和完成授权 | 需要可信验收时 |

```text
进入仓库
  → 读取 AGENTS.md：知道默认规则
  → 普通任务：直接做
  → 用户只要求交接：保存一个 Handoff
  → 用户启用 Summer：在 Lite / GSD 中选一个
  → 当前阶段再按需选择 Skill
  → 用真实验证决定能不能完成
```

仓库中的 [`config/AGENTS.md`](config/AGENTS.md) 是这套规则的单一维护源。根目录 `AGENTS.md` 只负责指向它，避免两份规则逐渐不一致。

## 为什么要这样设计

### 1. Direct-first：为了启动快

多数请求不需要项目管理系统。默认不创建文件、不扫描全仓、不启动后台进程，才能让小任务保持和普通 Codex 一样快。

### 2. 显式启用：为了不让工具替用户扩大流程

Agent 可以提醒“这个任务可能适合 Harness”，但不能因为觉得任务复杂就自动创建状态、拆阶段或启动一堆 Worker。是否进入 Harness 由用户决定。

### 3. 生命周期路由和 Skill 路由分开：为了避免双重路由

“任务应该 Direct、Lite 还是 GSD”和“当前阶段该用 TDD、Debug 还是 Review”是两个问题。`ask-matt` 可以在 Matt Skills 内选工具，但它不知道 Summer 的 Handoff、GSD Authority、多 Agent lease 或可信完成，因此不能作为第二个总路由器。

### 4. 重型项目交给 GSD：为了不重复造项目管理器

GSD 已经擅长需求澄清、Phase、Plan、Wave、fresh-context execution。Summer 不再复制一套 Task/Phase 系统，而是让 `.planning/` 继续拥有项目计划，自己只补充协调、Skill 选择、恢复入口和可信验收。

### 5. 只有一个 Handoff：为了让恢复没有歧义

聊天摘要、GSD 状态、Agent 记忆和多个 handoff 文件如果都声称“从我这里恢复”，新 Session 只能猜。Summer 规定 `.agent/HANDOFF.md` 是唯一公开入口；重型模式下它只指向 `.planning/` 当前阶段，不复制整份计划。

### 6. 并行写代码、串行写状态：为了防止多 Agent 分裂

多个 Worker 可以同时实现不同模块，但不能同时宣布任务完成或修改同一份计划。只有 Coordinator 接收并重新检查 Worker 的结果后，才能推进权威状态。

### 7. 有界恢复和 fresh context：为了对抗上下文腐烂

Summer 不把完整聊天当长期记忆。Handoff 只保留恢复真正需要的内容，Worker 只收到当前 Assignment，GSD 在阶段边界开启新上下文，大日志留在证据存储而不是塞进 Prompt。

### 8. Evidence-first：为了防止“嘴上完成”

Agent 写一句“测试通过”不算机器证据。验证必须绑定实际命令、当前代码版本和它能证明的范围；代码变了，旧结果就过期。

### 9. GUI 按需加载：为了不让产品壳变成本体

关系图和面板很有用，但它们不应该拖慢 Direct，也不应该成为第二套状态。未来只有运行 `summer ui` 时才加载 GUI 和查询数据库，删掉数据库也能从真实来源重建。

## 和其他 Harness 到底有什么区别

这些项目不是简单的“谁更强”，而是在解决不同层的问题。

| 方案 | 它最擅长什么 | 单独使用时缺什么 | Summer 如何处理 |
|---|---|---|---|
| **Superpowers** | Brainstorm、计划、TDD、Review 等完整编码仪式 | 很多小任务也会进入较完整流程；长期状态和多 Agent Authority 不是核心 | 不作为默认流程；需要的 TDD/Review 能力用窄 Skill 按需调用 |
| [GSD](https://github.com/open-gsd/gsd-core) | 把复杂目标拆成 Phase/Plan/Wave，并用 fresh context 执行 | 不负责所有 Direct 请求的外层启用规则，也不是通用 Evidence/Authority 控制面 | 直接作为重型 Workflow 后端，不复制它 |
| [Matt Skills](https://github.com/mattpocock/skills) | 小而专的 Debug、TDD、建模、Review 能力；`ask-matt` 能在其内部路由 | 不保存跨 Session 项目状态，不管理 GSD/Lite 切换，也不解决多 Agent 一致性和可信完成 | 作为 Capability 层直接选择具体 Skill，不再套一层 `ask-matt` 总路由 |
| [Missions](https://github.com/flowing-water1/Missions) | 强调 Claim Coverage、验证范围、production wiring 和独立 Review | 其 CSV/路由方式不适合作为 Summer 的唯一项目真相 | 吸收“声明必须被证据覆盖”的思想，不让 CSV 成为 Authority |
| [Harness Anything](https://github.com/FairladyZ625/harness-anything) | provenance、不可变记录、完成门禁、关系图和可重建投影 | 对小任务流程和概念偏重；它是治理/问责层，不是 Worker 调度器 | 只吸收 Evidence、Gate、provenance 和 projection；Direct 完全绕过治理层 |
| **gstack** | 产品、设计、QA、安全、发布等角色化 Skills | 角色和流程很多，容易和主 Workflow 重叠 | 只有用户点名具体 Skill 时才使用，不让其 session/checkpoint 成为 Summer 状态 |

一句话分工：

> **GSD 管复杂项目怎么拆，Matt/gstack Skills 管当前步骤怎么做，Host 管模型和 Worker 怎么运行，Git/CI 提供事实，Summer 管何时启用、从哪里恢复、多人如何不打架，以及凭什么算完成。**

这也是 Summer 相比“把多个 Harness 全装上”的主要优势：

- **轻**：普通任务零 Summer 状态；
- **快**：不默认加载 GSD、GUI、数据库和大套 Skills；
- **能持续**：一个 Handoff + 有界恢复，不依赖聊天记忆；
- **能扩展**：重型任务复用 GSD，不限制 Host 和模型；
- **多 Agent 一致**：Worker 并行，权威状态单写；
- **验收更可信**：完成绑定当前代码、证据、Review 和规则；
- **不锁死生态**：Skill、GSD、GUI 都可以替换，但不能抢走各自不该拥有的状态。

## 架构总览

> 下图描述目标 v3 架构，并不表示所有组件已经实现。

![Summer Harness v3 工作流架构](docs/diagrams/summer-harness-v3-workflow.svg)

[查看/下载交互式 Archify HTML 源文件（下载后本地打开）](docs/diagrams/summer-harness-v3-workflow.html) · [查看图表源 JSON](docs/diagrams/summer-harness-v3-workflow.workflow.json)

```text
用户请求
  → 未显式启用：Direct → 交付
  → 显式启用 Summer：Handoff Lite / Governed GSD 二选一
  → 当前阶段按需选择 Skill
  → Host / Workers 执行真实工作
  → Evidence + Review + Gate 决定能否完成
  → 需要跨 Session 时只从唯一 Handoff 恢复
```

## 三条使用路径

| 路径 | 人话解释 | 谁保存进度 |
|---|---|---|
| **Direct** | 直接让 Agent 做，不启动 Summer | 不保存 Summer 状态 |
| **Handoff Lite** | 一个目标顺序做，但需要换 Session 接着做 | `.agent/HANDOFF.md` |
| **Governed GSD** | 复杂目标拆阶段、拆依赖、多人并行 | GSD `.planning/`；Handoff 只保存恢复指针 |

多 Agent、多个活跃 Session、Phase、Wave 或 DAG 必须进入 GSD。Lite 只能显式晋升 GSD，不能让两边同时可写。

## 五层架构

```text
User / Codex / Claude
          │
          ▼
Explicit Activation Gate
          │
          ▼
Control Plane
Lifecycle Router / Capability Router / Coordinator
          │
          ▼
Workflow Plane
Handoff Lite XOR Governed GSD (.planning/)
          │
          ▼
Execution Plane
Host Workers / isolated worktrees / Git / CI
          │
          ▼
Trust Plane
Evidence → Execution → Review → GateReceipt
          │
          ▼
Continuity & Product Shell
Handoff / Resume Capsule / SQLite / Graph / on-demand GUI
```

### Control Plane

Activation Gate 先把未启用请求留在 Direct；只有明确启用 Summer 后，Lifecycle Router 才选择 Lite 或 GSD。随后确定唯一 Coordinator，并为当前阶段生成最小 SkillPlan。

### Workflow Plane

只描述“要做什么、如何拆分、当前做到哪里”。Lite 由 Handoff 拥有；重型项目由 GSD `.planning/` 拥有。Summer 不复制 GSD 的 Requirement、Phase、Plan、Wave 或 Task 状态。

### Execution Plane

Codex、Claude、GSD Worker、Git、测试和 CI 执行真实工作。Summer 不接管模型队列，不复制 Host 的 Worker scheduler。

### Trust Plane

将 Claim、Evidence、Execution、Review、GateReceipt 和 Authorization 绑定到当前 WorkRef、Workflow、Git tree、Evidence 集与 Policy，判断交付声明是否真的被证明。

### Continuity 与 Product Shell

提供唯一 Handoff、≤32 KiB Resume Capsule、Attention、搜索、关系图和按需 GUI。SQLite、FTS、Graph 和 GUI 都是可删除重建的 Projection，不是 Authority。

## 设计原则

- **Direct-first**：普通请求零 Summer 写入、零全仓扫描、零常驻进程。
- **Explicit activation**：只有用户明确说“使用 Summer Harness”“走 Harness”或调用 `$summer-harness` 才启用。
- **One lifecycle, one authority**：一个目标同时只能有一个 Workflow Authority。
- **Skills are capabilities, not lifecycles**：Skill 提供阶段能力，但不能写 Handoff、`.planning/` 或 Trust Journal。
- **Bounded continuity**：恢复读取有界 Capsule，而不是重放聊天或扫描全部历史。
- **Parallel execution, serialized authority**：代码可以并行，Workflow、Handoff 和 Trust acceptance 必须串行。
- **Evidence over assertion**：机器要求的 Gate 不能被手写“测试通过”满足。
- **Disposable projections**：GUI 和数据库只是视图，删除后必须可从 Authority 重建。
- **Human-approved evolution**：重复失败可以形成改进候选，但 Agent 不能自动修改 Policy、Skill、AGENTS 或代码。

## 每类事实只有一个 Owner

Summer 不建立“一个数据库拥有一切”的总账，而是划分清晰的 Authority：

| 事实 | 唯一 Owner |
|---|---|
| 代码、commit、tree、diff | Git |
| Lite 当前工作集 | `.agent/HANDOFF.md` |
| GSD Requirement / Phase / Plan / Wave / Task | `.planning/` |
| Evidence / Execution / Review / Gate / Authorization | append-only Trust Journal |
| 被引用过的 SkillPlan / GateSpec / Policy 原文 | content-addressed Contract Registry |
| 搜索、图、GUI、报表 | 可重建 Projection |

Summer 使用稳定 `WorkRef` 引用 Lite action 或 GSD entity，而不是复制另一份标题、进度和状态。这避免 Handoff、GSD、GUI 与 Trust Journal 同时维护四份“当前任务”。

## Capability Router：按阶段选择能力

Activation Gate 先决定请求保持 Direct 还是进入 Summer；进入 Summer 后，Lifecycle Router 只选 Lite 或 GSD。Capability Router 才决定当前 activity 需要什么 Skill。

每个目标 SkillPlan 包含：

- 一个 primary Skill；
- 最多两个 supporting Skills；
- `inline`、`fresh` 或 `parallel-wave` 执行策略；
- 预期 Artifact；
- 必需 Evidence 与 Gate；
- 可解释 route reasons；
- Skill version 与 content digest。

需要更多能力时拆分 Assignment，而不是加载一整套 Prompt 套餐。Matt Skills 只作为窄能力使用，例如 `grilling`、`domain-modeling`、`codebase-design`、`diagnosing-bugs`、`tdd` 和 `code-review`；`ask-matt` 不成为第二个 Router。

## 多 Agent 一致性

Worker 在独立 worktree 和 branch 中并行工作，只获得有界 Assignment Capsule，并提交 immutable Proposal。

只有 Coordinator 可以推进 `.planning/`、Handoff 和 Trust acceptance。接收 Proposal 时，Coordinator 必须重新计算并检查：

- base/head SHA 与真实 diff；
- changed paths 与路径授权；
- dependency、Wave readiness 和任务重叠；
- Workflow digest 与 fencing epoch；
- Evidence freshness 与 proof scope；
- Reviewer independence。

核心规则是：**并行写代码，串行写权威状态。**

首发只承诺同一 Git common-dir 下的本地一致性，不宣称跨机器分布式共识。

## 可信完成

Summer 将“完成”拆成一条可审计链：

```text
Claim / Acceptance
        ↓
Evidence
        ↓
Execution
        ↓
Independent Review
        ↓
GateReceipt
        ↓
CompletionAuthorization
        ↓
Exact terminal transition
```

Evidence 同时表达：

- **Capture trust**：真实进程捕获、CI attestation、文件摘要，还是人工说明。
- **Proof scope**：它实际证明 static、unit、integration、e2e、production wiring 或 external side effect 中的哪一层。

GateReceipt 只是 `verified / limited / failed` 的 evaluation，不直接拥有完成权限。只有绑定当前 WorkRef、Workflow、tree、Evidence set、SkillPlan、Gate Policy 和 exact successor 的 CompletionAuthorization，才能授权一次终态转换。

任一绑定发生变化，旧结果都必须变为 stale。`failed` 永不授权；`limited` 只有 Policy 明确允许，且 Host 提供模型无法伪造的用户确认时才可能继续。

## 如何防止上下文腐烂

Summer 不保存完整聊天，而是主动限制恢复输入：

- Handoff ≤4 KiB；
- Resume Capsule ≤32 KiB；
- `must_read` 最多五个 repo-relative 安全路径；
- GSD 在 Phase、Plan 和 Wave 边界使用 fresh context；
- Worker 只读取 Assignment Capsule，不继承完整主对话；
- 一个 SkillPlan 默认只含 1 primary +≤2 supporting Skills；
- raw logs 和大 Artifact 留在 Evidence Store，不进入 Prompt；
- Attention 只展示 blocker、drift、stale Evidence、pending Review/Proposal 和唯一下一步。

Handoff 因此不是“长期记忆数据库”，而是一个可靠的恢复启动扇区。

## 从其他系统吸收了什么

Summer 的目标是取长补短，不把几个重型 Harness 叠在一起。

| 来源 | 吸收 | 明确不采用 |
|---|---|---|
| [GSD](https://github.com/open-gsd/gsd-core) | `.planning/`、Discuss/Plan/Execute/Verify、Phase/Wave、fresh context | 不复制 GSD Workflow，不建立第二套 Task store |
| [Missions](https://github.com/flowing-water1/Missions) | Claim Coverage、proof scope、limited validation、production wiring、independent review | CSV 不作为 Authority，不采用 sticky router 或多个 Handoff |
| [Harness Anything](https://github.com/FairladyZ625/harness-anything) | provenance、immutable records、completion gates、rebuildable projections | 不为每个小改动建立全域实体，不启用常驻重型控制面 |
| [Matt Skills](https://github.com/mattpocock/skills) | 小而组合式的工程能力 | `ask-matt` 不作为第二 Router，Skill 不拥有生命周期 |
| Summer v2 | Go deep kernel、CAS、idempotency、fsync、recovery、migration | Native Objective/WorkItem 不再用于新的 v3 Workflow |

最重要的分工：**GSD 负责重型 Workflow，Matt Skills 负责阶段能力，Host 负责执行和 Worker 调度，Summer 负责入口、恢复、一致性与可信完成。**

## 当前实现状态

| 能力 | 状态 | 说明 |
|---|---|---|
| Native v2 `start/save/resume/doctor` | 已实现 | legacy compatibility，不再推荐给新工作 |
| transaction digest chain、revision CAS、Engine/Ledger-level idempotency | 已实现 | 当前 Go kernel 能力 |
| 本地跨进程写入串行化 | 已实现 | 支持的 Unix-like 平台；不是分布式锁 |
| Handoff/Snapshot rebuild、fault recovery | 已实现 | v2 continuity |
| v1→v2 dry-run / migration / rollback | 已实现 | 当前 `summer migrate` 范围 |
| `Engine.Execute` / machine Evidence | M2 在途 | 不在当前公开 HEAD 或可安装版本中 |
| Handoff Lite Go writer / v3 migration | 计划中 | M3/M4 |
| Lifecycle / Capability Router、Coordinator、Trust Gate | 计划中 | M3 |
| Governed GSD Adapter | 计划中 | M4 |
| 按需 GUI、SQLite/FTS/Graph Projection | 计划中 | M5 |
| Host Adapter、Controlled Evolution | 计划中 | M6 |
| `summer setup`、Release binary、Homebrew、桌面签名 | 计划中 | M7 |

目标命令、schema 和不变量已经写入 v3 规格，但不代表当前二进制已支持。

## 现在应该怎么使用

### 1. 普通任务：保持 Direct

不运行 Summer，不创建状态文件。直接让 Agent 完成问答、研究、审查、单点修复或常规开发；需要时只调用一个窄 Skill。

### 2. 只需要跨 Session：使用 Project Handoff

当前推荐通过 `$project-handoff` 保存或恢复唯一 `.agent/HANDOFF.md`。这不会启用完整 Harness。

Handoff Lite 是 v3 目标 Backend。当前过渡期由 `$project-handoff` 使用 legacy `mode=direct` helper 保存顺序快照；它不具备未来 Go Lite writer 的完整 CAS、SkillPlan 和 terminal Authorization 保证。

### 3. 重型或并发项目：直接使用 GSD

出现 Phase、Wave、DAG、多 Agent 或多个活跃 Session 时，让 `.planning/` 成为唯一 Workflow Authority。Handoff 只保存 pointer、digest、当前 Phase/Plan 和恢复命令。

在 Governed GSD Adapter 交付前，不要通过另一套 Summer Ledger 模拟 GSD；使用已安装的 GSD Skills 完成 Discuss、Plan、Execute 和 Verify。

### 4. 明确要求 Summer：调用 `$summer-harness`

Skill 会解释路由原因，选择 Lite 或 GSD 目标 Backend，并根据当前已实现能力继续或明确报告 capability gap。它不会自动把普通任务升级为 Harness。

### 5. 既有 Native v2 项目：仅兼容续接

```bash
summer resume
summer doctor
summer save \
  --done "<verified result>" \
  --next "<one action>" \
  --validation "<evidence>"
```

`summer save` 只允许已经授权、且尚未进入 migration fence 的既有 Native 在途工作。不要为新项目运行 `summer start` 创建 Native lifecycle，也不要手工修改 Ledger、HEAD、Snapshot、Handoff 或 migration archive。

当前 `--validation` 只是 checkpoint 文本，不是 machine Evidence；当前 `doctor` 主要检查 Native continuity 是否可读，不等同于未来完整的 Authority、Trust、Adapter 和 Projection 健康检查。

## 安装

### 环境要求

- Go 1.26 或更高版本；
- Git；
- 可选：Codex 或 Claude Code；
- 重型工作流另需安装兼容的 GSD Skills。
- 只有使用过渡期 Project Handoff helper 时才需要 Python 3。

### 作者电脑的 Codex 预览部署

仓库提供一个 **maintainer-only、可重复、fail-closed** 的本机部署脚本。它只管理三个软链接：全局 `AGENTS.md`、`summer-harness` 和 `project-handoff`；遇到同名真实文件或不同目标的链接会拒绝覆盖。随后检查 CLI、GSD/Matt/gstack 表面、显式启用规则、Handoff smoke 和旧 Harness 冲突。

```bash
python3 scripts/deploy_codex_preview.py --install
```

只读复检：

```bash
python3 scripts/deploy_codex_preview.py
python3 scripts/system_doctor.py
```

这不是尚未交付的正式 `summer setup codex`。它用于作者电脑和当前仓库开发预览，不安装 Go、GSD、Matt Skills 或 gstack；这些依赖缺失时会明确失败。

### 使用 Go 安装开发预览

仓库当前没有 Git tag 或正式 GitHub Release；`@latest` 会跟随仓库最新可用 revision，不等于稳定发行版。

```bash
go install github.com/summerchaserwwz/summer-harness/cmd/summer@latest
summer --version
```

当前预期输出：

```text
summer 0.1.0-dev
```

确保 Go bin 在 `PATH` 中：

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

### 从源码构建

```bash
git clone https://github.com/summerchaserwwz/summer-harness.git
cd summer-harness
mkdir -p bin
go build -o ./bin/summer ./cmd/summer
./bin/summer --version
```

### 手工安装 Codex Skills

在仓库根目录执行：

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"

ln -sfn "$PWD/skills/project-handoff" \
  "${CODEX_HOME:-$HOME/.codex}/skills/project-handoff"

ln -sfn "$PWD/skills/summer-harness" \
  "${CODEX_HOME:-$HOME/.codex}/skills/summer-harness"
```

推荐默认只安装 `project-handoff` 和 `summer-harness`。`adaptive-harness-router` 是 compatibility-only 解释器，不属于默认表面。

以下渠道尚未交付，请不要按稳定功能使用：

- `summer setup codex|claude`；
- GitHub Release binaries / checksums / SBOM；
- Homebrew formula；
- 签名和公证的桌面应用。

## 当前 CLI 表面

当前二进制只承诺 Native v2 compatibility：

```text
summer start <goal> [--next <text>] [--repo <path>] [--json]
summer save [--done <text>] [--next <text>] [--validation <text>] [...]
summer resume [--repo <path>] [--json]
summer migrate --dry-run [--repo <path>] [--json]
summer migrate [--repo <path>] [--json]
summer migrate --rollback [--repo <path>] [--json]
summer doctor [--repo <path>] [--json]
summer --version
```

`--lite`、`--gsd`、`route --explain`、`promote gsd`、`run --`、`check` 和 `ui` 是 v3 目标命令，尚未在当前 release surface 中实现。

## Migration 边界

当前 `summer migrate` 只实现 v1→v2：

```bash
summer migrate --dry-run
summer migrate
summer resume
summer doctor
```

目标 v2→v3 migration 需要零写 dry-run、原字节备份、semantic equivalence、CAS Handoff switch、persistent tombstone、crash recovery，并在首个 v3 write 前保留 rollback。该能力交付前，不能用手工移动或改写 Handoff/Ledger 代替。

## Roadmap

| Milestone | 状态 | 交付重点 |
|---|---|---|
| M0 | 完成 | v2 architecture baseline |
| M1 | 完成 | Go Engine/Ledger、continuity、v1→v2 migration |
| G0 | 完成 | v3 architecture、Authority、migration contract freeze |
| M2 | 进行中 | machine Evidence、Execution、Review、freshness |
| M3 | 计划中 | Lite/Capability Router、Coordinator、Gate/Authorization |
| M4 | 计划中 | Governed GSD Adapter、promotion、v2→v3 migration |
| M5 | 计划中 | 按需 GUI 与可重建 Projection |
| M6 | 计划中 | Host Adapter 与人工批准 Evolution |
| M7 | 计划中 | 安装、Release、Homebrew、桌面分发 |
| M8 | 计划中 | 开源发布材料、示例与完整 release evidence |

完整范围、验证命令和 stop-if 条件见 [Delivery Roadmap](docs/roadmap.md)。

## 开发与验证

```bash
go test ./internal/...
go test -race ./...
go vet ./...
python3 -m unittest tests.test_harnessctl -q
python3 scripts/system_doctor.py
python3 scripts/check_architecture_contract.py
```

验证 Archify 图表：

```bash
node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" \
  validate workflow docs/diagrams/summer-harness-v3-workflow.workflow.json --json

node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" \
  check docs/diagrams/summer-harness-v3-workflow.html
```

## 设计资料

- [v3 产品规格](docs/product-spec-v3.md)
- [v3 系统架构](docs/architecture-v3.md)
- [v3 数据模型](docs/data-model-v3.md)
- [领域语言](CONTEXT.md)
- [Authority Matrix](docs/architecture-v3.md#authority-matrix)
- [Route Table](docs/architecture-v3.md#route-table)
- [Delivery Roadmap](docs/roadmap.md)
- [威胁模型](docs/threat-model.md)
- [可交互 v3 系统架构图](docs/diagrams/summer-harness-v3.html)
- [Native v2 历史架构](docs/architecture-v2.md)

## 明确不接受

- 普通请求隐式启用 Harness；
- 为新工作创建 Native Objective/WorkItem；
- Handoff 与 `.planning/` 双写 GSD Task 状态；
- Skill、Worker、GUI、SQLite 或 Plugin 直接写 Authority；
- Summer 复制 Host Worker scheduler；
- CSV 或 Projection 成为 canonical state；
- Agent 审批自己的高风险 Review；
- 未经用户批准自动修改 Policy、Skill、AGENTS 或代码；
- 用 mock、fixture、dry-run 或文字说明冒充真实集成、E2E 或外部副作用 Evidence。

## License

[Apache License 2.0](LICENSE)
