# Summer Harness 架构

## 结论

Summer Harness 不是一个始终运行的“超级流程”，而是一个按需治理层：普通任务绕过它；复杂工作只有在用户显式授权后才进入。它把 Harness Anything 的持久状态原则、GSD 的 fresh-context 执行方式和 Matt Skills 的窄能力组合到一个单主账本模型中。

```text
用户请求
  |
  +-- 默认 ------------------------------> Direct
  |                                         + 可选一个 Matt Skill
  |
  +-- 只需跨 session --------------------> Direct + HANDOFF.md
  |
  +-- 显式要求 Harness
        |
        +-- 边界明确、需要审计 ----------> Summer native
        |                                   .agent/ledger/ + HANDOFF.md
        |
        +-- 真正多阶段、fresh-context ----> GSD backend
                                            .planning/ + HANDOFF pointer
```

## 为什么不全换 Matt Skills

Matt Skills 很适合做 bug 诊断、TDD、代码设计和领域建模等能力模块，但它们不提供稳定的任务主账本、完成门禁或唯一跨 session 状态。`ask-matt` 是面向用户调用的静态技能导航器，不应和全局路由并列。

因此 Summer 只安装并路由到少数能力：`ask-matt`、`diagnosing-bugs`、`codebase-design`、`domain-modeling`、`tdd`。生命周期仍由 Direct、Summer 或 GSD 三者之一独占。

gstack 也从 52 个全局入口收缩为 8 个按需能力：spec、CEO review、设计咨询/审查、browser、QA/QA-only 和 landing review。它们不维护项目生命周期。

## 为什么不直接使用完整 GSD

GSD 的优势是多阶段计划和每阶段 fresh context；劣势是完整 surface 很大，普通任务进入后会增加启动和治理成本。全局只保留 `standard` surface，需要更多能力时通过 `gsd-surface` 临时扩展，任务结束再收回。

GSD 模式不写 Summer Task：`.planning/` 是唯一主账本，`.agent/HANDOFF.md` 只记录当前状态文件和恢复命令。这一约束消除了双状态源。

## 记忆模型

- `Task`：目标、验收、状态、下一步、验证、阻塞、风险和审查。
- `Decision`：选了什么、拒绝了什么、依据是什么。
- `Fact`：带来源、置信度和时间的观察；只追加或失效。
- `HANDOFF`：当前工作集的有界投影，不是第四种账本。

恢复胶囊总量不超过 32 KiB，最多带入五个 must-read、三个近期 Decision 和十二个有效 Fact；超出预算时优先丢弃较旧的 Fact/Decision 并报告 omitted 数量。聊天记录、探索性推理、重复源码和全量历史不会进入恢复上下文。

## 可靠性边界

当前实现针对个人、本地、单协调者场景：原子替换与目录 fsync、进程所有权锁、Task/Handoff 微型 write-ahead transaction、摘要/投影一致性检查、修订绑定验证和 fail-closed 完成门。GSD pointer 只能指向仓库内 `.planning/` 文件并绑定内容摘要。以下情况出现后再升级，而不是预先背负成本：

- 多 Agent 同时写同一账本：增加写入队列/coordinator。
- 数百任务需要复杂查询：增加可重建 SQLite projection。
- 大型二进制证据：增加内容寻址存储。
- 要求外部副作用或多系统 exactly-once 重放：增加完整 event journal + watermark。

这些升级都不得改变 Markdown/Git 为权威状态、投影可删除重建、一个任务只有一个生命周期所有者的原则。

## 使用口令

- 普通任务：直接说需求。
- 使用窄能力：`用 diagnosing-bugs 排查这个问题`。
- 只保存：`保存交接，下个 session 继续`。
- 启用原生治理：`使用 Summer Harness 完成……`。
- 强制多阶段：`使用 Summer Harness，GSD 后端完成……` 或直接调用 `$gsd-new-project`。
- 恢复：`恢复工作` 或 `$project-handoff`。
