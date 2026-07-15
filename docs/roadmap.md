# Summer Harness v3 Delivery Roadmap

每个 Milestone 使用独立 `/goal`，范围、验收和停止条件可机械审计；单个建议预算不超过 300,000 tokens。Milestone closeout 只是 checkpoint，不能替代目标验收。

## 状态

| Milestone | 状态 | 说明 |
|---|---|---|
| M0 | completed | v2 architecture baseline |
| M1 | completed | Go Engine/Ledger、continuity、v1→v2 migration 已推送 |
| G0 | completed | v3 architecture、authority 与 migration contract 已固化并通过独立 Review |
| M2 | in-progress | machine Evidence vertical slice 存在未提交 WIP |
| M3–M8 | planned | 需独立 Goal |

## G0 — v3 Architecture Freeze

**Deliverables**

- `architecture-v3.md`、`product-spec-v3.md`、`data-model-v3.md`。
- ADR 0005–0007、Authority Matrix、Route Table、migration matrix。
- AGENTS、Skills、README、Threat Model、diagram 和 contract checker 一致。

**Verification**

```bash
python3 scripts/check_architecture_contract.py
node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" validate architecture docs/diagrams/summer-harness-v3.architecture.json --json
node "${CODEX_HOME:-$HOME/.codex}/skills/archify/bin/archify.mjs" check docs/diagrams/summer-harness-v3.html
git diff --check
```

**Stop-if**：需要修改生产代码、丢失 Native v2 数据或产生双 Authority。

## M2 — Trusted Delivery

**Goal boundary**：只交付 `Execute`、machine Evidence、immutable Execution/Review 和 WorkRef migration；不实现 Gate evaluation/Authorization、Capability Router、GSD orchestration 或 GUI。

**Dependencies**：G0；现有 M2 WIP 与 Trust Journal schema。

**Deliverables**

- `summer run -- <argv>` direct argv capture。
- repository-bound cwd/path、environment allowlist、stream redaction、content-addressed objects。
- Evidence trust/proof scope、WorkRef/workflow/tree binding。
- immutable Execution、Review 与 workflow/tree/evidence-set stale detection。
- Native v2 Evidence→WorkRef migration tests。

**Verification**

```bash
go test -race -count=1 ./internal/evidence ./internal/engine ./internal/cli
go vet ./...
python3 -m unittest tests.test_harnessctl -q
python3 scripts/system_doctor.py
```

**Stop-if**：实现需要 shell interpretation、原始 secret 落盘、弱化当前 Evidence tests 或让 manual attestation 满足 machine-required Gate。

**Suggested budget**：180,000 tokens。

## M3 — Lifecycle/Capability Router and Coordinator

**Goal boundary**：交付 Lite/GSD route policy、SkillManifest/SkillPlan 和本地单 Coordinator；不实现完整 GSD write Adapter 或 GUI。

**Dependencies**：M2 WorkRef/Trust receipts。

**Deliverables**

- explicit Activation Gate、`summer start` 自动 route、`--lite|--gsd` override 和 `route --explain` target contract。
- Handoff Lite sequential CAS、独立 working-set digest、terminal successor、Completion/Cancellation Authorization consumption、`LiteCheckpointAccepted`、pending/crash recovery 和 parallel→GSD hard trigger。
- Capability Router：1 primary +≤2 support、effects、context budget、route reasons；Lite Handoff Plan 与 GSD immutable ActivitySkillPlan。
- immutable/content-addressed Contract Registry：SkillManifest、SkillPlan snapshot、GateSpec set、Gate Policy bytes、version/digest lookup。
- Gate evaluation、GateReceipt、Completion/Cancellation Authorization generator、Policy/Plan/Gate freshness。
- `TrustedUserInteractionProvider` 与至少一个 production-supported、模型工具不可伪造的最小 Host/local user channel，用于 Contract activation、limited acceptance 和 cancellation；无该边界返回 `USER_ACCEPTANCE_UNAVAILABLE`。
- Git common-dir fencing lease、Assignment Capsule、Proposal inbox 与 digest-bound accepted/rejected receipts。

**Verification**

```bash
go test -race -count=1 ./internal/continuity ./internal/capability ./internal/contract ./internal/trust ./internal/approval ./internal/collaboration ./internal/cli
go vet ./...
```

**Stop-if**：Skill 成为状态 Owner、锁只在单 worktree 可见、Lite 接受并行 Writer、Gate digest 无法反查 exact contract bytes 或需要第二 Task store。

**Suggested budget**：280,000 tokens。

## M4 — Governed GSD Adapter

**Goal boundary**：让 `.planning/` 成为可治理的重型 Workflow Authority；不复制 GSD 或实现 Host scheduler。

**Dependencies**：M3 Coordinator/SkillPlan；可用且受支持的 GSD schema/version。

**Deliverables**

- GSD inspect/prepare/accept Adapter 与 compatibility matrix。
- WorkflowSnapshot digest、staged exact successor、terminal Gate authorization、pending marker、checkpoint saga、reconcile。
- Lite→GSD promotion saga：common-dir fence、Lite freeze、target preimage/CAS、semantic validation、Handoff switch、tombstone 和 partial-failure recovery。
- Worker Summary/Proposal 由 Coordinator 串行写入 `.planning`。
- `start --gsd`、`promote gsd`、GSD Handoff pointer target contract。
- Native v2→GSD dry-run/apply/rollback migration。

**Verification**

```bash
go test -race -count=1 ./internal/gsd ./internal/collaboration ./internal/continuity ./internal/cli
go vet ./...
```

**Stop-if**：未知 GSD version、无法阻止双写、只能通过静默 fallback 到 Lite 或无法证明 crash recovery 的唯一后继。

**Suggested budget**：260,000 tokens。

## M5 — On-demand GUI and Projections

**Goal boundary**：交付 `summer ui` 产品壳和可重建 View；不加入常驻 daemon。

**Dependencies**：M2 Trust、M4 GSD Adapter。

**Deliverables**

- React/Vite + Go embed + loopback random port/token。
- Resume、Workflow、Coverage、Evidence、Agents、Evolution、Graph、Health/Settings。
- SQLite/FTS/Graph projection、incremental refresh、full rebuild。
- 中文默认 UI、VoltAgent 密度、键盘和基础可访问性。

**Verification**

```bash
npm --prefix ui ci
npm --prefix ui run typecheck
npm --prefix ui test -- --run
npm --prefix ui run build
go test -race -count=1 ./internal/ui ./internal/projection ./...
```

**Stop-if**：GUI 写 SQLite/Markdown、Direct/Resume 加载 Node/SQLite、Projection 删除后不可重建或 loopback security tests 失败。

**Suggested budget**：280,000 tokens。

## M6 — Host Adapters and Controlled Evolution

**Goal boundary**：交付 Codex/Claude/GSD Host mapping 与人工批准 Evolution；不接管模型调度。

**Dependencies**：M3 Assignment、M4 GSD、M5 UI approval surface。

**Deliverables**

- Host capability/Actor/Session mapping、Capsule export、Proposal/Evidence ingest。
- 扩展 Codex/Claude Host mapping 与 trusted user-interaction adapters；M3 的最小 provider继续兼容。Evolution approval 和 global second confirmation 使用不同 InteractionID，模型/Coordinator 不可伪造。
- runtime status projection；queue/budget/cancel 仍由 Host 拥有。
- `candidate -> approved|rejected -> applied -> verified|rolled_back`。
- source refs、counterexamples、diff、validation、rollback 与 global second confirmation。
- dogfood 一个真实 Candidate。

**Verification**

```bash
go test -race -count=1 ./internal/adapter ./internal/evolution ./internal/collaboration
go vet ./...
```

**Stop-if**：Adapter 直接写 Authority、Evolution 自动批准、外部内容绕过 provenance 或 Summer 启动自主模型重试循环。

**Suggested budget**：240,000 tokens。

## M7 — Installation and Desktop

**Goal boundary**：可重复安装、打包和桌面分发；不以未签名开发包冒充正式产物。

**Dependencies**：M2–M6 Release Gate。

**Deliverables**

- `summer setup codex|claude` 幂等安装，隔离 HOME tests。
- GitHub Release binaries、checksums、SBOM、Homebrew formula。
- Wails shell 复用 Engine/UI。
- macOS signing/notarization workflow；Windows/Linux CI 和安装说明。

**Verification**

```bash
./scripts/release_gate.sh --local
./scripts/install_smoke.sh
go test -race -count=1 ./...
```

**Stop-if**：需要覆盖用户配置、缺少正式签名凭据却声称 notarized、release bundle 包含个人路径/secret 或依赖许可证不兼容。

**Suggested budget**：260,000 tokens。

## M8 — Open Source Release

**Goal boundary**：公开资料、示例、发布证据和项目治理；不把一次 push 当作 Release。

**Dependencies**：M7 signed/reproducible artifacts。

**Deliverables**

- 中英文 README、quickstart、architecture、security、contributing、migration、recovery。
- 示例项目、真实 GUI 截图/录屏、公众号素材。
- GitHub Actions：unit、race、contract、fault、GUI、cross-platform、release smoke。
- public Release receipt、known gaps 和 residual risks。

**Verification**

```bash
./scripts/release_gate.sh
gh release view "$VERSION"
brew install summerchaserwwz/tap/summer
summer --version
summer doctor
```

**Stop-if**：GitHub/Homebrew/Apple credentials 不可用、remote 不符、需要 force push、公开资产含 secret/个人路径/不可再分发内容。

**Suggested budget**：220,000 tokens。

## Cross-Milestone Release Gates

- machine Evidence 绑定当前 WorkRef/workflow/tree/evidence-set。
- 独立 Review 绑定当前 Execution，不能由同一高风险 contributor/session 自审。
- Handoff/Resume/migration/Projection/install 有真实 smoke 或 fault Evidence。
- Direct、Resume、UI 和 1,000-entity query 达到产品预算。
- secret、绝对个人路径、license 和不可再分发资产扫描无 high-confidence finding。
