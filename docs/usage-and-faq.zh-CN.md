# Summer Harness 使用与常见问题

这篇文档只回答实际使用中的问题：什么时候需要 Harness，跨 Session 怎么接，多套 Harness 怎么收拢，Codex 和 Claude 同时工作时怎么避免互相覆盖。

如果只想记住一条原则，就是：

> 简单任务直接做；需要接班时留一张小纸条；多人并行时只设一个项目经理。

当前仓库是 `v0.1.0-dev`。文中会把“现在可用”和“v3 目标能力”分开写。没有交付的能力不会按现成功能介绍。

## Harness 到底是什么

Coding Agent 会写代码，但它天然有几个弱点：

- 聊天越长，越容易忘记早期约束；
- 换一个 Session 后，新窗口不知道前面做到哪里；
- 多个 Agent 同时开发时，可能覆盖代码、计划或完成状态；
- Skill 越装越多，几个路由器可能同时接管同一个任务；
- Agent 很容易把“看起来没问题”说成“已经完成”。

Harness 不是一个更聪明的模型，而是模型外面的工作系统。它规定从哪里恢复、谁能改状态、什么时候用什么能力，以及一项工作凭什么算完成。

Summer Harness 只保留三条工作路径：

| 情况 | 路径 | 状态放在哪里 |
|---|---|---|
| 问答、研究、Review、小修复、普通开发 | Direct | 不创建 Summer 状态 |
| 一个目标顺序推进，只是需要换 Session | Handoff Lite | `.agent/HANDOFF.md` |
| 分阶段、依赖图、并行 Agent、多个活跃 Session | Governed GSD | `.planning/`；Handoff 只保存恢复指针 |

任务复杂不等于自动启用 Summer。只有你明确说“使用 Summer Harness”“走 Harness”或调用 `$summer-harness`，它才进入完整流程。单纯说“保存交接”只使用 `$project-handoff`，不会启动 GSD。

## 跨 Session：需要手动加锁吗

不需要创建锁文件，也不需要手动执行锁命令。

安全用法是：

1. 在旧 Session 中说“保存交接”。
2. 等它确认保存完成。
3. 让旧 Session 停止修改项目。
4. 在新 Session 中说“恢复工作”。

保存过程会短暂获得一个写锁，检查当前状态，然后更新 `.agent/HANDOFF.md`，写完立即释放。这个锁只保护一次写入，不会从旧 Session 一直锁到新 Session。

### Handoff 保存什么

Handoff 是接班纸条，不是聊天备份。它只保存：

- 当前目标；
- 已验证的完成项；
- 唯一下一步；
- 阻塞；
- 验证结果；
- 最多五个必须读取的仓库内文件。

新 Session 先读适用的 `AGENTS.md` 和 `git status`，确实需要恢复时才读取 Handoff。它不应该重放完整聊天，也不应该默认扫描全部 `.planning/` 和历史日志。

### 为什么不能直接依赖聊天记录

聊天可能被压缩、截断，或者只存在于某个产品和某个 Session。更麻烦的是，聊天里经常同时存在已经过期的计划、推测和真实结果。Handoff 只留下当前仍然有效的工作集，因此恢复输入更小，也更不容易把旧结论当成事实。

### 当前版本能防住什么

当前过渡期 `$project-handoff` helper 已经具备：

- 同一仓库目录、同一台机器上的短时写入串行化；
- Handoff 大小和安全路径检查；
- GSD 指针摘要检查；
- 原子写入和基础漂移检测。

它还不具备目标 Handoff Lite writer 的完整 revision CAS，也没有跨 worktree 的 Git common-dir lease。

这意味着它能防止两个进程在同一瞬间把一个文件写坏，但不能阻止旧 Session 在稍后用过期内容再次覆盖新 Session。不同 worktree 也是不同目录，不能把当前短时锁当作共享项目锁。

所以当前必须遵守：

> Handoff Lite 只允许一个仍在工作的 Session。旧 Session 保存后必须停止写入；只要两个 Session 需要同时工作，就进入 GSD。

### v3 准备怎样解决过期写入

目标 Handoff Lite 会为每次保存绑定预期 revision：

```text
Session A 读取 revision 8
Session B 把状态更新到 revision 9
Session A 再拿 revision 8 保存
→ 拒绝：你看到的状态已经过期
```

多 Agent 模式则使用 Git common-dir 中的 Coordinator lease 和 fencing epoch。每次修改权威状态前都重新检查 epoch，失去 Coordinator 身份的旧 Session 即使恢复运行，也不能继续推进计划。

这些属于 M3 目标能力，当前版本不能假装已经提供。

## Handoff 和 GSD `.planning/` 有什么区别

它们不能同时拥有同一份项目状态。

| 内容 | 谁负责 |
|---|---|
| 一个顺序目标的当前工作集 | `.agent/HANDOFF.md` |
| 重型项目的 Requirement、Phase、Plan、Wave、Task | GSD `.planning/` |
| 重型项目从哪里恢复 | Handoff 中的一条 `.planning/` 指针 |
| 代码、diff、commit | Git |
| 测试和交付证明 | Evidence；当前阶段仍以真实命令、CI 和 Git 结果为准 |

进入 GSD 后，Handoff 不再复制任务列表。它只回答：“当前应该读 `.planning/` 的哪一部分，用什么命令继续。”

因此 Handoff 不是多 Agent 消息队列。Worker 之间通过 Git commit 或提交提案（Proposal）交付，Coordinator 通过 `.planning/` 管理工作流，只有 Coordinator 换 Session 时才更新 Handoff。

## 已经使用别的 Harness，怎么迁移

迁移的重点不是把旧目录全部搬进 Summer，而是先确定以后谁有权解释“现在做到哪里”。

### 第一步：列出旧系统拥有哪些状态

重点检查：

- 它是否保存 Task、Phase 或 Plan；
- 它是否生成自己的 Handoff 或 Session 状态；
- 它是否会自动 checkpoint、提交或修改 Issue；
- 它只是一个 Skill 集，还是一套完整生命周期；
- 当前是否还有旧 Session 在写这些文件。

### 第二步：只选择一个工作流状态来源

| 旧系统 | 推荐处理方式 |
|---|---|
| GSD | 保留 `.planning/`，直接作为重型项目的唯一状态来源 |
| Superpowers | 停用默认全流程；需要的 TDD、Review 等能力改为按需 Skill |
| Matt Skills | 继续作为 Debug、TDD、建模、Review 等窄能力，不迁移生命周期 |
| gstack | 只保留你明确点名的具体 Skill；停用它的 Session、checkpoint、Issue 或自动提交状态 |
| 其他带 Task/Handoff 的 Harness | 先冻结旧写入，再迁移当前有效工作集；旧目录改为只读历史 |
| Summer Native v2 | 先用 `summer doctor`、`summer resume` 兼容恢复；正式 v2→v3 migration 交付前不要手工改 Ledger |

### 第三步：只迁移仍然有效的内容

应该迁移：

- 当前目标；
- 已验证的完成项；
- 唯一下一步或当前 GSD Phase；
- 阻塞；
- 仍然有效的关键决策；
- 能找到真实来源的测试和交付证据。

不应该迁移：

- 完整聊天和模型思考过程；
- 已经过期的计划；
- 重复的 Task 副本；
- 大段日志和源码副本；
- 旧 Harness 的自动路由和自动 checkpoint 规则；
- 无法核验来源的“已经通过”说明。

### 第四步：停止旧系统继续写入

迁移结束后必须做到：

```text
一个工作流状态来源
一个公开恢复入口
一个 Coordinator
一套完成判定
```

旧 Harness 可以留作历史查询，但不能继续更新当前状态。不要让旧 Harness、GSD 和 Handoff 同时维护三份“下一步”。

### 已有 GSD 项目怎么接入

这是最简单的情况：

1. 保留原有 `.planning/`。
2. 不运行新的 Native `summer start`。
3. 让 `.planning/` 继续拥有 Phase、Plan 和 Task。
4. 用 `$project-handoff` 保存一条 GSD 恢复指针。
5. 后续每个阶段按需使用 TDD、Debug、Review 等 Skill。

Governed GSD Adapter 尚未交付，因此当前不要再用一套 Summer Ledger 模拟 GSD 状态。

### 已有 Summer Native v2 项目怎么处理

先检查再恢复：

```bash
summer doctor
summer resume
```

只有已经授权的在途 Native 工作才继续使用兼容 `summer save`。当前 `summer migrate` 只覆盖 v1→v2，不是 v2→v3。正式迁移需要备份、CAS 切换、持久 migration fence 和崩溃恢复；交付前不要手工移动 `.agent/ledger` 或重写 Handoff 冒充迁移。

## Codex 和 Claude 同时开发，怎么保证能接起来

两个模型可以同时写代码，但不能同时当项目经理。

最稳妥的分工是：

```text
你
└── Coordinator（例如 Codex 主 Session）
    ├── Codex Worker：独立 worktree / branch
    └── Claude Worker：独立 worktree / branch

Worker 返回 commit 和验证结果
Coordinator 重新检查、串行合并、更新 .planning/
```

只有 Coordinator 可以：

- 修改 `.planning/**`；
- 修改 `.agent/HANDOFF.md`；
- 接受或拒绝 Worker 的结果；
- 推进 Phase、Plan 和最终状态；
- 完成最终合并和收口验证。

Worker 可以：

- 修改 Assignment 明确允许的代码；
- 运行测试；
- 在自己的分支提交；
- 返回 commit SHA、修改路径、验证结果和已知问题。

### 每个 Worker 使用独立 worktree

不要让 Codex 和 Claude 同时操作同一个 working tree。示例：

```bash
git worktree add ../project-codex-worker -b codex/worker-api
git worktree add ../project-claude-worker -b claude/worker-ui
```

主目录留给 Coordinator，两个 Worker 各自只使用自己的目录和分支。任务完成并合并后，再按正常 Git 流程清理 worktree。

### Assignment 要写清楚边界

给 Worker 的任务至少包含：

```text
任务：实现登录限流
基准提交：<base SHA>

允许修改：
- internal/auth/**
- tests/auth/**

禁止修改：
- .agent/**
- .planning/**
- AGENTS.md
- CLAUDE.md

验收：
1. 超过限制后返回 429
2. 现有认证测试无回归
3. 新增限流测试

交付：
1. commit SHA
2. 修改文件列表
3. 实际执行的验证命令和结果
4. 已知问题与残余风险
```

任务之间尽量分配不同路径。确实需要修改同一个文件时，放在有先后依赖的不同 Wave，不要并行写。

### Worker 完成后，Coordinator 怎么接

Coordinator 不直接相信 Worker 的完成说明，而是：

1. 确认 Worker 的 base 和 head SHA；
2. 重新查看真实 diff；
3. 检查有没有修改禁止路径；
4. 检查依赖是否仍然成立；
5. 串行 cherry-pick 或 merge；
6. 在合并后的代码上重新运行必要验证；
7. 验证通过后才更新 `.planning/` 和 Handoff。

Agent 可以并行产生代码，但计划、Handoff 和验收状态必须串行更新。这是多 Agent 一致性的核心。

### Codex 和 Claude 怎样读取同一套规则

Codex 读取项目 `AGENTS.md`。Claude Code 使用项目 `CLAUDE.md`，并支持用 `@path` 导入共享文件。可以在项目根目录放一个很薄的桥接文件：

```md
@config/AGENTS.md

# Claude Worker 约束

- 默认作为 Worker，不是 Coordinator。
- 禁止修改 `.agent/**` 和 `.planning/**`。
- 只能修改 Assignment 允许的路径。
- 必须通过 Git commit 提交结果。
- 返回 commit SHA、验证命令、结果和已知风险。
```

这样 Codex 和 Claude 最终引用同一份规则，不需要维护两套逐渐漂移的项目约束。Claude Code 的项目指令和导入行为见 [Anthropic 官方文档](https://docs.anthropic.com/zh-CN/docs/claude-code/memory)。

当前作者电脑只完成了 Codex 预览部署。正式 `summer setup claude`、Claude Host Adapter 和可信用户交互 Adapter 属于 M6/M7；在它们交付前，Claude 的 Worker 权限主要靠项目指令、独立 worktree 和 Coordinator 复核约束，不能宣称已经由 Summer Engine 强制执行。

### Coordinator 也要换 Session 怎么办

先停止旧 Coordinator，再保存 GSD Handoff。新 Coordinator Session 从 Handoff 指向的 `.planning/` 状态恢复，然后继续接收 Worker 结果。

当前版本不要让两个 Coordinator 同时活跃。未来 common-dir lease 和 fencing epoch 会支持显式接管：新 Coordinator 获得新 epoch 后，旧 Coordinator 的权威写入会被拒绝。

## 常见误解

### “任务很复杂，Summer 会不会自动启动？”

不会。复杂度只能让 Agent 建议使用 Summer，不能替你授权。你明确说“使用 Summer Harness”后才进入 Lite 或 GSD。

### “调用一个 Skill，是不是已经进入 Harness？”

不是。Direct 任务可以临时调用 `$tdd`、`$diagnosing-bugs` 或 `$code-review`。Skill 是能力，不拥有项目生命周期。

### “Handoff 是不是长期记忆数据库？”

不是。它只保存恢复当前工作的最小信息。长期架构决策应该进入项目文档或 ADR，代码历史进入 Git，重型计划进入 GSD。

### “保存 Handoff 后，旧窗口还能继续工作吗？”

当前不建议。旧窗口继续工作会成为过期 Writer。要继续并行，就改用 GSD，并明确唯一 Coordinator。

### “能不能让 Codex 和 Claude 都当 Coordinator，提高容错？”

不能同时当。两个 Coordinator 会产生两份互相竞争的计划和完成判定。容错依靠显式接管，不依靠双写。

### “Codex 和 Claude 能不能共享一个工作目录？”

不建议。即使 Git 最终能提示冲突，未提交文件、格式化、生成物和测试缓存也会互相干扰。每个 Worker 使用独立 worktree。

### “GSD 自己已经能跨 Session，为什么还要 Handoff？”

GSD 保存重型项目内部状态；Handoff 提供统一恢复入口。新 Session 先读一个很小的 Handoff，再按指针进入当前 GSD Phase，不需要猜应该扫描哪个 `.planning` 文件。Handoff 不复制 GSD，只负责指路。

### “Harness Anything 的账本是不是更完整，为什么不全搬过来？”

完整账本适合高审计需求，但对普通任务成本过高。Summer 只吸收 provenance、Evidence、Gate 和可重建视图；Direct 完全绕过这些机制，重型任务才逐步增加治理。

### “现在已经能完全保证 Codex + Claude 不冲突吗？”

还不能机械保证。现在依靠 GSD 单一状态源、一个 Coordinator、独立 worktree、Git commit 和复核流程安全协作。自动 common-dir lease、Proposal ingest、GSD Adapter 和 Claude Adapter 仍在 Roadmap。

## 设计思想从哪里来

Summer 没有把现有 Harness 简单叠加，而是只吸收各自最有价值的部分：

| 来源 | 吸收的部分 | 没有照搬的部分 |
|---|---|---|
| [GSD](https://github.com/open-gsd/gsd-core) | Phase、Plan、Wave、fresh context、`.planning/` | 不再复制一套重型 Task 系统 |
| [Matt Skills](https://github.com/mattpocock/skills) | 小而专的 Debug、TDD、建模和 Review 能力 | 不让 `ask-matt` 成为第二个生命周期路由器 |
| [Harness Anything](https://github.com/FairladyZ625/harness-anything) | provenance、不可变记录、Evidence、完成门禁、可重建视图 | 不要求每个小任务建立完整治理账本 |
| [Missions](https://github.com/flowing-water1/Missions) | Claim Coverage、验证范围、独立 Review | 不让 CSV 或另一个 Router 成为项目真相 |
| Git | commit、tree、diff、branch、worktree | 不用聊天摘要代替代码事实 |
| Summer v2 | CAS、幂等、事务、恢复和迁移经验 | Native Objective/WorkItem 不再承接新的 v3 Workflow |

最后形成四条约束：

1. **不用时没有成本**：普通任务保持 Direct。
2. **同一种事实只有一个主人**：Git 管代码，Handoff 管 Lite，GSD 管重型 Workflow。
3. **执行可以并行，决定必须串行**：Worker 并行，Coordinator 单写权威状态。
4. **完成必须能被证明**：真实验证、当前代码和交付声明必须对得上。

## 当前能力和目标能力

| 能力 | 现在 | 目标版本 |
|---|---|---|
| 普通任务 Direct-first | 已通过项目规则使用 | 保持 |
| 顺序跨 Session | `$project-handoff` 过渡 helper 可用 | 原生 Lite writer、revision CAS |
| 同目录短时写锁 | 可用 | 保持为本地写入保护 |
| 跨 worktree Coordinator lease | 未实现 | M3 |
| GSD 项目使用 | 直接使用已安装 GSD Skills | M4 Governed GSD Adapter |
| Codex + Claude 并行 | worktree + 单 Coordinator 手工治理 | M6 Host Adapter 与 Proposal ingest |
| machine Evidence | M2 在途，尚未公开交付 | M2 |
| Gate / CompletionAuthorization | 未实现 | M3 |
| 正式安装器和 Claude setup | 未实现 | M7 |

完整里程碑见 [Delivery Roadmap](roadmap.md)，架构不变量见 [Summer Harness v3 Architecture](architecture-v3.md)。

## 一页操作卡

```text
普通任务
→ 直接做

旧 Session 停止，新 Session 接手
→ 保存交接
→ 等保存完成
→ 停止旧 Session
→ 新 Session 恢复工作

两个 Agent 或两个 Session 同时工作
→ 使用 GSD
→ 指定唯一 Coordinator
→ 每个 Worker 使用独立 worktree
→ Worker 只交 commit 和验证结果
→ Coordinator 复核并串行合并

已有旧 Harness
→ 先冻结旧 Writer
→ 选择唯一 Workflow Authority
→ 只迁当前有效状态
→ 旧状态保留为只读历史
```
