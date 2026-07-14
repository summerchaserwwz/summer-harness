# Summer Harness Domain

Summer Harness 是一个本地优先的 Coding Agent 连续性与治理系统。它让会话和 Worker 可以随时替换，同时保留可恢复、可验证、可审查的项目状态。

## Language

**Root Objective**:
一个项目当前唯一的顶层交付目标，定义结果、验收条件和生命周期所有者。
_Avoid_: Epic, master task, project task

**WorkItem**:
Root Objective 下可独立分配、验证和审查的一块工作；多个 WorkItem 可以并行，但不能拥有第二套项目生命周期。
_Avoid_: Subtask, child task, phase task

**Actor**:
对 Harness 发出命令或提交记录的身份，可以是 User、Coordinator、Worker、Reviewer 或 System。
_Avoid_: Account, operator

**Coordinator**:
唯一有权分配 WorkItem、接收 Worker Proposal、推进 Root Objective 和执行完成转换的 Actor。
_Avoid_: Main agent, manager agent, master

**Worker**:
在被分配的 WorkItem 和代码范围内执行工作的 Actor；Worker 不能直接修改 Root Objective 或权威 Handoff。
_Avoid_: Subagent, child agent

**Assignment**:
Coordinator 授予 Worker 的有界工作授权，包含代码范围、基线、分支、worktree、验收条件和租约。
_Avoid_: Dispatch, job ticket

**Proposal**:
Worker 交给 Coordinator 的不可变工作提案，包含代码提交、变更范围、Evidence、已知缺口和残余风险。
_Avoid_: Progress update, worker result

**Execution**:
针对某个 WorkItem 修订提交的一次不可变交付声明，绑定 deliverables、Git 状态和 Evidence 集合。
_Avoid_: Run, attempt, result

**Evidence**:
对可观察事实的带来源证明，具有明确的捕获方式、信任等级、内容摘要和适用修订。
_Avoid_: Validation text, proof note

**Review**:
Reviewer 针对特定 Execution、代码树和 Evidence 集合给出的不可变判定。
_Avoid_: Approval note, feedback

**Fact**:
经过来源标注的项目观察；Fact 只能追加或失效，不能无痕改写。
_Avoid_: Note, assumption

**Decision**:
对难以逆转的项目问题所选择的方案，以及被拒绝方案和依据。
_Avoid_: Preference, note

**Evolution Candidate**:
由重复失败、Review finding 或 Fact 模式产生的惰性改进建议；未经 User 批准不会改变 Policy、Skill 或代码。
_Avoid_: Auto-fix, learned rule

**Policy**:
经过 User 批准、具有版本和内容摘要的 Harness 行为约束。
_Avoid_: Prompt, suggestion

**Canonical Ledger**:
决定项目治理状态的唯一、Git 可追踪记录；任何数据库、看板和摘要都不能覆盖它。
_Avoid_: Database, cache, dashboard state

**Projection**:
由 Canonical Ledger 派生、可删除重建的读取模型，例如 SQLite、搜索索引、关系图和页面摘要。
_Avoid_: Source of truth, primary store

**Handoff**:
跨 Session 的唯一有界恢复入口，只包含当前焦点、唯一下一步、验证状态和必须读取文件。
_Avoid_: Session transcript, full memory

**Attention**:
由当前状态派生的待处理优先级，包括阻塞、过期 Evidence、待 Review、漂移、Worker Proposal 和 Evolution Candidate。
_Avoid_: Dashboard summary, notification list
