# Summer Harness

[中文](README.md) | [English](README_EN.md)

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/License-Apache--2.0-34d399)](LICENSE)
[![Status](https://img.shields.io/badge/status-v0.1.0--dev-f59e0b)](docs/roadmap.md)

**Summer Harness v3 是一套显式启用、local-first、GSD-backed、trust-centered 的 Coding Agent 控制面。**

简单任务保持 Direct；单工作流跨 Session 使用一个轻量 Handoff；Phase、Wave、DAG、多 Agent 或多个活跃 Session 交给 GSD；Summer 只补充能力路由、一致性保护、可信交付和有界恢复。

它不是模型、Prompt 大礼包、通用项目管理器、Worker 调度器，也不是第二套 GSD。

> [!IMPORTANT]
> 当前仓库是 **`v0.1.0-dev` 开发预览 / v3 architecture freeze**。GitHub HEAD 已交付 Native v2 continuity、CAS、digest chain、恢复和 v1→v2 migration；这些能力现已进入 legacy compatibility。M2 Machine Evidence 正在开发，但不属于当前公开 HEAD 或可安装版本；Handoff Lite Go writer、Governed GSD Adapter、Capability Router、Trust Gate、GUI、Host Adapter 和安装器属于 Roadmap，尚未作为稳定产品发布。

## 架构总览

> 下图描述目标 v3 架构，并不表示所有组件已经实现。

![Summer Harness v3 工作流架构](docs/diagrams/summer-harness-v3-workflow.svg)

[查看/下载交互式 Archify HTML 源文件（下载后本地打开）](docs/diagrams/summer-harness-v3-workflow.html) · [查看图表源 JSON](docs/diagrams/summer-harness-v3-workflow.workflow.json)

主流程只有一条：

```text
用户请求
  → Activation Gate：未显式启用 → Direct → 交付
  → 显式启用 Summer → Lifecycle Router：Handoff Lite / Governed GSD
  → Capability Router：按当前阶段选择最小 SkillPlan
  → Host / Workers 执行真实工作
  → Evidence → Review → Gate
  → 交付结果，或保存唯一 Handoff 后有界恢复
```

## 为什么需要它

Coding Agent 通常不缺更多 Prompt，真正缺少的是几个工程边界：

1. **简单任务被重流程拖慢**：小改动也初始化完整 Harness，启动成本和上下文成本高于收益。
2. **跨 Session 无法可靠续接**：聊天会压缩、截断或腐烂，新 Session 不知道可信状态和唯一下一步。
3. **多 Agent 产生分裂状态**：多个 Worker 同时修改计划、Handoff 或完成状态，最终没有权威答案。
4. **Skill 越装越多，上下文越用越差**：整套方法论被一次性加载，真正任务被路由和流程文本淹没。
5. **“完成”只是文字声明**：测试、Review、代码树、Workflow 和实际 Claim 范围没有绑定，旧验证会被错误复用。

Summer 的目标不是让所有工作变重，而是在用户明确需要时，提供最小但可靠的控制面。

## 三条生命周期路径

每个请求只有一个生命周期；Skill 只是能力叠加，不会创造第四条路径。

| 路径 | 适用场景 | Workflow Authority | 状态成本 |
|---|---|---|---|
| **Direct** | 问答、研究、审查、小修复和常规开发 | 无 | 零 Summer 状态 |
| **Handoff Lite** | 一个目标、顺序执行、需要跨 Session | `.agent/HANDOFF.md` | 一个 ≤4 KiB 当前工作集 |
| **Governed GSD** | Phase、Wave、DAG、长期 Roadmap、多 Agent、多个活跃 Session | `.planning/` | GSD Workflow + Summer 治理 |

关键路由规则：

- 默认永远是 Direct；复杂度只能建议，不能替用户启用 Summer。
- `Direct + Skill` 仍然是 Direct。
- Handoff Lite 只支持顺序 Writer，不支持 Phase graph 或并发工作流。
- 多 Agent、多个活跃 Session、Phase、Wave 或 DAG 是 GSD 硬触发。
- Lite 只能显式、单向晋升 GSD；Lite 与 GSD 不能同时可写，GSD 不能静默降级。
- 高风险不等于重型 Workflow：顺序完成的高风险小任务可以使用 Lite，但需要更严格的 Evidence、Review 和 Gate。

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

### 可选：安装 Codex Skills

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
