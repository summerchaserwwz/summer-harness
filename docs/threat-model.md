# Summer Harness Threat Model

## 保护目标

1. Canonical Ledger 的完整性和单一性。
2. Handoff 的真实性、边界和可恢复性。
3. Evidence 与当前代码树、修订和执行环境的绑定。
4. Worker 不越过 Assignment、代码范围和生命周期权限。
5. Evolution 不被提示注入、偶然失败或恶意内容永久污染。
6. GUI、Projection、Plugin 和 Runner 不成为绕过 Kernel 的写路径。
7. 开源发布不泄露 secret、完整会话、个人路径或敏感 Artifact。

## 信任边界

```text
User confirmation                  高信任，但可能误操作
Kernel + canonical transaction     权威边界
Local process Evidence             可证明 Harness 启动并观察进程
CI attestation                     取决于 CI 身份与 artifact 保留
Worker Proposal                    不可信输入，必须重新验证
GUI / SQLite / graph               可重建 Projection，不可信写源
Plugin / external Skill / web      不可信建议或数据
Local malicious process / OS root  超出本地 Harness 可完全防御范围
```

## Threats and Mitigations

### Ledger tampering or torn writes

风险：半提交、手改文件、并发写入或磁盘中断让状态分叉。

措施：单 Writer、expected revision、进程所有权锁、transaction directory、前驱摘要、事件摘要、fsync、原子 HEAD、orphan 检测和 `doctor` fail-closed。Git 是额外审计，不是唯一一致性机制。

### Handoff poisoning

风险：攻击者或旧 Session 修改目标、下一步或 must-read，引导新 Session 执行错误内容。

措施：Handoff 由 Canonical Ledger 投影并绑定 ledger head/source digest；Native 模式禁止反向覆盖；must-read 必须 repo-relative、存在、非 symlink；4KiB/5 文件上限；漂移时拒绝恢复并提供 repair。

### Fake completion evidence

风险：Agent 手写“测试通过”，实际没有运行，或 Evidence 对应旧代码树。

措施：代码/发布 Gate 要求 observed process 或合格 CI attestation；记录 argv、exit、时间、Git HEAD、dirty digest、artifact digest；Evidence、Execution、Review 绑定 revision/tree/evidence-set，任何变化使旧 Review 失效。

### Shell injection

风险：Evidence Runner 把未经信任的字符串交给 shell。

措施：默认使用 argv 直接执行，不隐式调用 shell；显式 shell 模式要求用户确认并在 Receipt 标记；cwd 必须仓库内；环境变量使用 allowlist。

### Secret leakage in logs and Git

风险：stdout、stderr、命令参数、环境或截图包含 token、密钥和个人信息。

措施：高置信 secret scanner、redaction、大小限制、敏感参数标记、环境 allowlist、内容寻址私密存储、Git allowlist、发布前 secret scan。发现疑似 secret 时 fail-closed，不把原始内容写入公开 Ledger。

### Worker scope escape and confused deputy

风险：Worker 修改未授权路径、基于错误 SHA 工作、冒充其他 Assignment，或让 Coordinator 错误 ingest。

措施：独立 worktree/branch、base SHA、allowed paths、capability hash、lease、proposal digest；Coordinator ingest 时重新读取 Git diff 和文件路径，不信任 Worker 自报；Worker 无权 complete 或修改 Handoff。

### Reviewer self-approval

风险：执行者换一个字符串身份给自己批准。

措施：Review 绑定 Actor/Session/contributors；高风险任务要求 reviewer session 与 execution sessions 不同。早期版本明确这是 provenance 约束，不宣传为密码学身份；后续可以加入签名或 host-authenticated Actor Adapter。

### Evolution poisoning

风险：网页、日志、Issue 或 Agent 输出中的提示注入被提炼成永久 Policy/Skill。

措施：Candidate 必须有多个 source refs 或高影响证据、反例、影响范围、diff、验证和 rollback；默认 inert；只有 User 能批准；全局 scope 二次确认；应用后必须产生 machine Evidence；失败自动建议 rollback，但不能静默修改历史。

### Projection corruption or GUI split-brain

风险：SQLite/GUI 显示 Canonical Ledger 不存在的状态，或 GUI 直接写缓存。

措施：Projection 保存 ledger head/revision/projector version/digest；所有写入穿过 Engine；不匹配时重建；GUI 页面展示当前 projection cursor；删除缓存后必须能恢复相同视图。

### Loopback GUI attack

风险：浏览器其他页面向本地 GUI 发请求，或局域网访问未授权界面。

措施：只绑定 `127.0.0.1`/`::1` 随机端口、启动时短期 bearer token、严格 Origin、SameSite cookie 或 header token、CSP、禁止任意文件 URL、敏感操作二次确认。默认关闭远程监听。

### Plugin supply-chain compromise

风险：第三方 Plugin 直接执行代码或写 Ledger。

措施：默认不扫描、不自动加载 Plugin；manifest、版本和 digest 固定；Plugin 只能产生 Proposal/Gate result/Projection extension；Kernel 重新验证；未来优先进程隔离 JSON-RPC/WASI，而不是直接 import。

### Runner runaway cost or destructive action

风险：内建 Worker 无限重试、消耗预算或执行破坏性命令。

措施：显式并发、预算、超时、重试上限、取消、allowed commands/path policy；破坏性和外部副作用继续由宿主权限系统控制；Runner 终止不影响 Ledger 恢复。

## Non-goals

- 不防御已完全控制本机内核、用户账号或磁盘的攻击者。
- 不把 local Evidence 宣传成远程可信执行证明。
- 首发不实现企业多租户 RBAC、分布式共识或远程公共 daemon。
- 不允许为了安全幻觉把 Direct 路径变成常驻监控系统。

## Release Security Gates

- secret scan 对仓库、Git history、release bundle 和示例项目无高置信发现。
- symlink/path traversal、transaction crash、revision race、Handoff tamper、projection drift 和 Worker scope escape 均有故障注入测试。
- GUI 通过 loopback、Origin、token、CSP 和 CSRF 测试。
- Evidence redaction 使用固定测试向量并验证原始 secret 未落盘。
- release binary 提供 checksums、SBOM；正式桌面发布增加签名和 macOS notarization。
- 发布扫描拒绝仓库内出现开发者绝对路径或只能在单机成立的全局配置。
