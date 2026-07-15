---
status: accepted
supersedes:
  - data-model-v2.md
---

# Summer Harness v3 Data Model

本文件定义 v3 目标序列化契约。当前 v0.1 类型仍是兼容实现；新 schema 必须通过后续 Milestone 的 TDD 和 migration tests 才能成为生产能力。

## Design Rules

1. Workflow state 与 Trust state 分域。
2. GSD 实体只通过 `WorkRef` 引用，不在 Summer Journal 镜像 Task status。
3. Evidence、Execution、Review、GateReceipt、CompletionAuthorization 和用户授权回执 immutable。
4. 每个 record 绑定 source revision/digest；绑定变化产生新 record 或 stale，不覆盖历史。
5. Handoff 是有界恢复入口，不是 Trust Journal 或 GSD 的副本。

## Canonical Encoding and Base Types

跨 Go、Host Adapter、GSD Adapter 和未来 GUI 的 digest 使用同一规则：

1. schema id 必须进入被 hash 内容。
2. 数据编码为 UTF-8 canonical JSON：object key 按 Unicode code point 升序、无无意义空白、整数使用十进制、禁止 NaN/Infinity。
3. 字符串在 hash 前做 Unicode NFC；repo path 使用 `/`、`path.Clean` 等价规范化，禁止绝对路径、`.`、`..` 和 symlink escape。
4. 时间统一 UTC RFC3339Nano；duration 使用整数毫秒。
5. 有语义顺序的数组保留顺序；声明为 set 的字段先去重再按 canonical element bytes 升序。v3 set fields 至少包括 `AllowedPaths`、`RequiredProofScope`、`EvidenceRefs`、`ProofScopes`、`Contributors`、`RequiredScopes`、`AllowedKeys` 和 `ChangedPaths`；Acceptance、Reasons、Gaps 和 migration Mapping 保留声明顺序。
6. 计算一种 record 的 digest 时，只把该 schema 明确指定的顶层 self-digest 字段（例如 GateReceipt.Digest 或 Evidence.ManifestDigest）置空；嵌套 `ContentRef.Digest`、WorkRef.SourceDigest 等事实摘要必须保留。使用 `SHA-256(schema + "\n" + canonical_json)` 小写 hex。
7. ContentRef digest 只 hash redacted/stored bytes；raw secret bytes 永不进入 canonical encoder。
8. 未知字段在同一 schema version 中拒绝；schema upgrade 通过显式 upcaster，不静默丢字段。

稳定基础类型：

```go
type ActorRef struct {
    ActorID   string `json:"actor_id"`
    SessionID string `json:"session_id"`
    Runtime   string `json:"runtime"`
    Model     string `json:"model,omitempty"`
    Role      string `json:"role"` // user|coordinator|worker|reviewer|system
}

type SkillRef struct {
    SkillID       string `json:"skill_id"`
    Version       string `json:"version"`
    ContentDigest string `json:"content_digest"`
}

type GateSpec struct {
    GateID          string   `json:"gate_id"`
    RequiredTrust   string   `json:"required_trust"`
    RequiredScopes  []string `json:"required_scopes"` // canonical set
    ReviewPolicy    string   `json:"review_policy"`
    WiringRequired bool     `json:"wiring_required"`
    LimitedAllowed bool     `json:"limited_allowed"`
}

type UserInteractionProof struct {
    Channel          string `json:"channel"` // trusted-host-ui|trusted-host-approval
    InteractionID    string `json:"interaction_id"`
    ChallengeDigest  string `json:"challenge_digest"`
    HostReceiptDigest string `json:"host_receipt_digest"`
}

type TrustedUserInteractionReceipt struct {
    ReceiptID        string    `json:"receipt_id"`
    Purpose          string    `json:"purpose"` // limited-acceptance|cancel|evolution-approve|evolution-confirm|policy-approve
    Scope            string    `json:"scope"` // global|project|work
    ProjectID        string    `json:"project_id,omitempty"`
    WorkIdentity     *WorkIdentityRef `json:"work_identity,omitempty"`
    TargetDigest     string    `json:"target_digest"`
    RequestDigest    string    `json:"request_digest"`
    User             ActorRef  `json:"user"` // provenance only
    Proof            UserInteractionProof `json:"proof"`
    RecordedAt       time.Time `json:"recorded_at"`
    Digest           string    `json:"digest"`
}

type HandoffSkillPlan struct {
    Primary            SkillRef   `json:"primary"`
    Supporting         []SkillRef `json:"supporting"` // max 2
    AgentStrategy      string     `json:"agent_strategy"`
    ExpectedArtifact   string     `json:"expected_artifact"`
    EvidencePlanDigest string     `json:"evidence_plan_digest"`
    RequiredGateDigests []string  `json:"required_gate_digests"`
    Reasons            []string   `json:"reasons"`
    ContextBudget      int        `json:"context_budget"`
    Digest             string     `json:"digest"`
}

type EvidenceTrust string
type ProofScope string

type ObjectRef struct {
    Kind   string `json:"kind"`
    Ref    string `json:"ref"`
    Digest string `json:"digest"`
}

type ContentRef struct {
    Digest         string `json:"digest"`
    MediaType      string `json:"media_type"`
    OriginalBytes  int64  `json:"original_bytes"`
    StoredBytes    int64  `json:"stored_bytes"`
    Truncated      bool   `json:"truncated"`
    RedactionCount int    `json:"redaction_count"`
    Retention      string `json:"retention"`
}

type ExactBackupRef struct {
    ManifestDigest string `json:"manifest_digest"`
    ObjectDigest   string `json:"object_digest"`
    OriginalBytes  int64  `json:"original_bytes"`
    StoredBytes    int64  `json:"stored_bytes"`
    EntryCount     int    `json:"entry_count"`
    Retention      string `json:"retention"` // durable-until-rollback-closed
}

type EnvironmentSummary struct {
    AllowedKeys []string `json:"allowed_keys"` // sorted canonical set; values forbidden
    Tool        string   `json:"tool,omitempty"`
    ToolVersion string   `json:"tool_version,omitempty"`
    Digest      string   `json:"digest"`
}

type Finding struct {
    FindingID  string `json:"finding_id"`
    Severity   string `json:"severity"` // high|medium|low
    SourceRef  string `json:"source_ref"`
    EvidenceRef string `json:"evidence_ref"`
    Summary    string `json:"summary"`
    FollowUp   string `json:"follow_up,omitempty"`
}

type MigrationMapping struct {
    SourceRef    string `json:"source_ref"`
    TargetRef    string `json:"target_ref"`
    SourceDigest string `json:"source_digest"`
    TargetDigest string `json:"target_digest"`
    SemanticDigest string `json:"semantic_digest"`
}
```

`EvidenceTrust` 合法值：`observed_process|ci_attestation|file_digest|external_reference|manual_attestation`。

`ProofScope` 合法值：`static|unit|integration|e2e|production_wiring|external_side_effect`。

## WorkRef

```go
type WorkRef struct {
    ProjectID     string `json:"project_id"`
    Backend       string `json:"backend"`        // lite|gsd
    ExternalID    string `json:"external_id"`    // lite action id or GSD phase/plan/task ref
    SourcePath    string `json:"source_path"`     // .agent/HANDOFF.md or .planning relative path
    SourceRevision uint64 `json:"source_revision,omitempty"`
    SourceDigest  string `json:"source_digest"`
}
```

`WorkRef` 是引用，不拥有外部实体的标题、状态、依赖或进度。`SourceDigest` 变化使绑定的 SkillPlan、Execution、Review 和 GateReceipt stale。

```go
type WorkIdentityRef struct {
    ProjectID  string `json:"project_id"`
    Backend    string `json:"backend"` // lite|gsd
    ExternalID string `json:"external_id"`
}
```

`WorkIdentityRef` 是跨 revision 稳定 identity；`WorkRef.Identity()` 必须等于这三个字段。长期 Contract approval scope 使用 WorkIdentityRef，不能使用会变化的 SourceRevision/SourceDigest；Evidence、Execution、Review、Gate 和 stale detection 继续绑定完整 WorkRef。

## RouteDecision

```go
type RouteDecision struct {
    Schema       string   `json:"schema"`        // summer.route/v3
    Mode         string   `json:"mode"`          // direct|lite|gsd
    Reasons      []string `json:"reasons"`
    HardTriggers []string `json:"hard_triggers"`
    Explicit     bool     `json:"explicit"`
    DecidedAt    time.Time `json:"decided_at"`
}
```

未显式启用时 `Mode=direct`，且 RouteDecision 不必持久化。GSD hard triggers：`parallel_agents`、`multiple_active_sessions`、`phase_graph`、`dependency_wave`、`explicit_gsd`。

## Handoff V3

```go
type Handoff struct {
    Schema             string   `json:"schema"` // summer.handoff/v3
    Mode               string   `json:"mode"`   // idle|lite|gsd|legacy-direct|legacy-native
    ProjectID          string   `json:"project_id,omitempty"`
    ActionID           string   `json:"action_id,omitempty"`
    Goal               string   `json:"goal,omitempty"`
    SourceRef          string   `json:"source_ref,omitempty"`
    SourceRevision     uint64   `json:"source_revision,omitempty"`
    SourceDigest       string   `json:"source_digest,omitempty"`
    WorkingSetDigest   string   `json:"working_set_digest,omitempty"`
    CurrentWorkRef     *WorkRef `json:"current_work_ref,omitempty"`
    CurrentSkillPlan   *HandoffSkillPlan `json:"current_skill_plan,omitempty"`
    ActionState        string   `json:"action_state,omitempty"` // active|blocked|in_review|completed|cancelled
    Done               []string `json:"done,omitempty"`
    NextAction         string   `json:"next_action,omitempty"`
    Blockers           []string `json:"blockers,omitempty"`
    Validation         []string `json:"validation,omitempty"`
    LastVerifiedCommit string   `json:"last_verified_commit,omitempty"`
    MustRead           []string `json:"must_read,omitempty"`
    ResumeCommand      string   `json:"resume_command"`
    CompletionAuthorizationDigest string `json:"completion_authorization_digest,omitempty"`
    CancellationAuthorizationDigest string `json:"cancellation_authorization_digest,omitempty"`
    ContentDigest      string   `json:"content_digest"`
}
```

容量：encoded file≤4,096 bytes；`MustRead`≤5；derived Capsule≤32 KiB。非终态 Lite 必须恰有一个 `NextAction`；`completed|cancelled` 的 exact successor 必须清空 `NextAction`。`completed` 保存 CompletionAuthorization digest；`cancelled` 保存独立 CancellationAuthorization digest，二者不得互换。Lite `CurrentSkillPlan` 是当前 activity 的紧凑 canonical Plan，不是 Trust Journal 副本；保存前必须通过容量 preflight，跨 Session 必须恢复相同 Skill/version/digest。`legacy-native` 不接受新 save，只能 resume/doctor/migrate。旧 `mode=direct` 作为 `legacy-direct` 读取；v3 Writer 保存时升级为 `mode=lite`。

Lite 不允许让 `WorkRef.SourceDigest` 指向包含该 WorkRef 的完整 Handoff digest。`WorkingSetDigest` 使用独立 `summer.lite-working-set/v1` payload，且只覆盖 `ProjectID`、`ActionID`、`SourceRevision`、`Goal`、`ActionState`、`Done`、`NextAction`、`Blockers`、`Validation`、`LastVerifiedCommit` 和 `MustRead`；明确排除 `CurrentWorkRef`、`CurrentSkillPlan`、`WorkingSetDigest`、`CompletionAuthorizationDigest`、`CancellationAuthorizationDigest` 和 `ContentDigest`。Lite `CurrentWorkRef.ProjectID == Handoff.ProjectID`、`ExternalID == ActionID` 且 `SourceDigest == WorkingSetDigest`；Lite 的 Handoff `SourceRef/SourceDigest` 留空，它们只用于 GSD/legacy 外部 source pointer。Handoff `ContentDigest` 再按通用规则覆盖完整 Handoff（仅清空顶层 `ContentDigest`），因此两类摘要都可一次计算且没有自引用。

## WorkflowSnapshot

```go
type WorkflowSnapshot struct {
    Backend        string    `json:"backend"` // gsd
    AdapterVersion string    `json:"adapter_version"`
    PlanningPath   string    `json:"planning_path"`
    CurrentPhase   string    `json:"current_phase,omitempty"`
    CurrentPlan    string    `json:"current_plan,omitempty"`
    Workstream     string    `json:"workstream,omitempty"`
    Revision       uint64    `json:"revision,omitempty"`
    Digest         string    `json:"digest"`
    ObservedAt     time.Time `json:"observed_at"`
}
```

Summer 不保存一份可变 GSD Task list；GUI 需要时由 Adapter 读取 `.planning/` 并投影。

## SkillManifest

```go
type SkillManifest struct {
    SkillID          string   `json:"skill_id"`
    Version          string   `json:"version"`
    ContentDigest    string   `json:"content_digest"`
    Capabilities     []string `json:"capabilities"`
    ApplicableStages []string `json:"applicable_stages"`
    RequiredInputs   []string `json:"required_inputs"`
    ProducedArtifacts []string `json:"produced_artifacts"`
    Effects          []string `json:"effects"` // readonly|workspace_write|external_write
    ContextCost      int      `json:"context_cost"`
    FreshContext     string   `json:"fresh_context"` // preferred|required|forbidden
    RiskSupport      []string `json:"risk_support"`
    IncompatibleWith []string `json:"incompatible_with"`
    EvidenceKinds    []string `json:"evidence_kinds"`
    StateOwner       string   `json:"state_owner"` // must be none
}
```

缺失/未知 effect、越权路径或 `StateOwner != none` 的 Skill 不可自动调用。

## Contract Registry

Skill/Gate/Policy digest 不能成为无法反查原文的“孤儿 hash”。Immutable、content-addressed Contract Registry 保存每个曾被使用的 SkillManifest bytes、SkillPlan snapshot、规范化 GateSpec set 和 Gate Policy bytes；Engine 是唯一正式 Writer。更新创建新 version/ref，不覆盖旧对象。

```go
type ContractRef struct {
    Kind          string `json:"kind"` // skill-manifest|skill-plan|gate-set|gate-policy
    ContractID    string `json:"contract_id"`
    Version       string `json:"version"`
    ContentDigest string `json:"content_digest"`
    ObjectRef     ObjectRef `json:"object_ref"`
}

type GateContract struct {
    ContractID       string      `json:"contract_id"`
    Version          string      `json:"version"`
    Scope            string      `json:"scope"` // global|project|work
    ProjectID        string      `json:"project_id,omitempty"`
    WorkIdentity     *WorkIdentityRef `json:"work_identity,omitempty"`
    SkillManifestRefs []ContractRef `json:"skill_manifest_refs"`
    GateSpecs        []GateSpec  `json:"gate_specs"`
    RequiredGateSetDigest string `json:"required_gate_set_digest"`
    PolicyRef        ContractRef `json:"policy_ref"`
    PolicyDigest     string      `json:"policy_digest"`
    RecordedAt       time.Time   `json:"recorded_at"`
    Digest           string      `json:"digest"`
}

type ApprovedContractRecord struct {
    ApprovalID       string    `json:"approval_id"`
    CandidateContractDigest string `json:"candidate_contract_digest"`
    Scope            string    `json:"scope"` // global|project|work
    ProjectID        string    `json:"project_id,omitempty"`
    WorkIdentity     *WorkIdentityRef `json:"work_identity,omitempty"`
    InteractionReceiptDigest string `json:"interaction_receipt_digest"`
    ApprovedAt       time.Time `json:"approved_at"`
    Digest           string    `json:"digest"`
}

type SkillPlanContract struct {
    PlanDigest      string      `json:"plan_digest"`
    WorkRef         WorkRef     `json:"work_ref"`
    CanonicalPlanRef ContractRef `json:"canonical_plan_ref"`
    ArchivedAt      time.Time   `json:"archived_at"`
    Digest          string      `json:"digest"`
}
```

Engine 必须能由 GateReceipt 的 Plan/Gate/Policy digest 反查 exact canonical bytes。Lite/GSD SkillPlan 在首次被 Evidence、Execution 或 Gate 引用前归档为 immutable `SkillPlanContract`；它只保存历史合同，不成为第二个可变 Plan Authority。

Contract approval 使用两阶段对象以避免摘要环：先计算不含 approval 引用的 immutable Gate/Skill candidate digest；trusted receipt 的 `TargetDigest` 绑定该 candidate；Engine 再写 `ApprovedContractRecord(candidate, receipt)`。所有会被 Gate 或权限决策使用的 active Contract 都要求 trusted-user interaction；不提供未定义的 baseline shortcut。

Candidate、TrustedUserInteractionReceipt、ApprovedContractRecord 的 `Scope/ProjectID/WorkIdentity` 必须完全相等：`global` 禁止 ProjectID/WorkIdentity，`project` 必须 ProjectID 且禁止 WorkIdentity，`work` 必须同时携带 ProjectID 和匹配的 WorkIdentity。GateReceipt 只能引用与当前 `WorkRef.Identity()` scope-compatible 的 Approval（global、同 Project 或 exact WorkIdentity）。`ProjectContractDigest(project)` 只投影匹配该 Project/WorkIdentity 的 candidate/approval/SkillPlanContract，global Contract 明确排除。Registry 对象不可被 Projection、Skill 或 Coordinator 直接改写。

重复摘要必须满足强制等式并在 write/read 两侧 fail-closed：`ContractRef.ContentDigest == ContractRef.ObjectRef.Digest`；`CanonicalPlanRef.Kind == skill-plan`；`SkillPlanContract.PlanDigest == CanonicalPlanRef.ContentDigest`；归档 SkillPlan bytes 内的 WorkRef 与外层 WorkRef 完全相等；`GateContract.PolicyRef.Kind == gate-policy` 且 `PolicyDigest == PolicyRef.ContentDigest`；`RequiredGateSetDigest == canonical_digest(GateSpecs)`；SkillManifestRefs 的 kind/digest 与 exact bytes 一致。

## SkillPlan

```go
type SkillPlan struct {
    Schema         string     `json:"schema"` // summer.skill-plan/v1
    WorkRef        WorkRef    `json:"work_ref"`
    Stage          string     `json:"stage"`
    Primary        SkillRef   `json:"primary"`
    Supporting     []SkillRef `json:"supporting"` // max 2
    AgentStrategy  string     `json:"agent_strategy"` // inline|fresh|parallel-wave
    ExpectedArtifact string   `json:"expected_artifact"`
    EvidencePlan   []GateSpec `json:"evidence_plan"`
    RequiredGates  []GateSpec `json:"required_gates"`
    Reasons        []string   `json:"reasons"`
    ContextBudget  int        `json:"context_budget"`
    Digest         string     `json:"digest"`
}
```

Primary 必须恰好一个；Supporting≤2。需要更多能力时拆 Assignment。Router 不维护自己的 Task log；Lite 使用 Handoff.CurrentSkillPlan。GSD 的每个 routed activity 都写一条 immutable `ActivitySkillPlan`，Worker activity 再由 Assignment 引用同一 Plan digest；Discuss、Plan、Coordinator inline integration、fresh reviewer 和 final review 也必须有 Activity record，不能因为没有 branch/worktree 就丢失路由决定。Trust record 只能引用 Plan digest 和实际使用结果，不能保存第二份可变 Plan。

```go
type ActivitySkillPlan struct {
    ActivityID      string    `json:"activity_id"`
    ActivityKind    string    `json:"activity_kind"` // discuss|plan|worker|integration|review|verify|release
    WorkRef         WorkRef   `json:"work_ref"`
    WorkflowDigest  string    `json:"workflow_digest"`
    Owner           ActorRef  `json:"owner"`
    SkillPlan       SkillPlan `json:"skill_plan"`
    RecordedAt      time.Time `json:"recorded_at"`
    Digest          string    `json:"digest"`
}
```

`ActivitySkillPlan` 位于 append-only Coordination namespace，不拥有 Requirement/Phase/Task 状态。Assignment 只引用或内嵌内容完全相同的 Plan；digest 不同即拒绝发放。

## CoordinatorLease

```go
type CoordinatorLease struct {
    ProjectID  string    `json:"project_id"`
    Owner      ActorRef  `json:"owner"`
    Epoch      uint64    `json:"epoch"` // monotonic fencing token
    AcquiredAt time.Time `json:"acquired_at"`
    RenewedAt  time.Time `json:"renewed_at"`
    ExpiresAt  time.Time `json:"expires_at,omitempty"`
    Digest     string    `json:"digest"`
}
```

物理位置基于 Git common-dir，而非单个 worktree。Takeover 必须观察旧 epoch 终止或过期，并使用 CAS 创建更高 epoch。每次 Authority commit 前重读 lease，要求 owner/epoch 当前且未过期；transaction/checkpoint 保存 fencing epoch，并拒绝低于 current epoch 的 Writer。Renew 使用 owner+epoch CAS；本地单调时钟决定 TTL，wall-clock 仅用于持久诊断。

## AssignmentCapsule

```go
type AssignmentCapsule struct {
    AssignmentID      string    `json:"assignment_id"`
    WorkRef           WorkRef   `json:"work_ref"`
    WorkflowDigest    string    `json:"workflow_digest"`
    Owner             ActorRef  `json:"owner"`
    BaseCommit        string    `json:"base_commit"`
    Branch            string    `json:"branch"`
    Worktree          string    `json:"worktree"`
    AllowedPaths      []string  `json:"allowed_paths"`
    Acceptance        []string  `json:"acceptance"`
    SkillPlan         SkillPlan `json:"skill_plan"`
    RequiredProofScope []string `json:"required_proof_scope"`
    MustRead          []string  `json:"must_read"`
    FencingEpoch      uint64    `json:"fencing_epoch"`
    IssuedAt          time.Time `json:"issued_at"`
    IssuedBy          ActorRef  `json:"issued_by"`
    Digest            string    `json:"digest"`
}
```

AssignmentCapsule 作为 immutable `AssignmentIssued` record 写入 Coordination namespace；状态不靠改字段，而由 `issued -> submitted -> accepted|rejected|expired` receipts 派生。Allowed paths 和 MustRead 必须 repo-relative、规范化、无 symlink escape。Worker 不获得 canonical write capability。

## Proposal

```go
type Proposal struct {
    Schema        string    `json:"schema"` // summer.proposal/v1
    ProposalID    string    `json:"proposal_id"`
    IdempotencyKey string   `json:"idempotency_key"`
    AssignmentID  string    `json:"assignment_id"`
    WorkRef       WorkRef   `json:"work_ref"`
    Actor         ActorRef  `json:"actor"`
    FencingEpoch  uint64    `json:"fencing_epoch"`
    BaseCommit    string    `json:"base_commit"`
    HeadCommit    string    `json:"head_commit"`
    ChangedPaths  []string  `json:"changed_paths"`
    DiffDigest    string    `json:"diff_digest"`
    EvidenceRefs  []string  `json:"evidence_refs"`
    Deliverables  []ObjectRef `json:"deliverables"`
    KnownGaps     []string  `json:"known_gaps"`
    ResidualRisks []string  `json:"residual_risks"`
    SubmittedAt   time.Time `json:"submitted_at"`
    Digest        string    `json:"digest"`
}
```

Proposal immutable。Worker 先把 envelope fsync 到 common-dir inbox；inbox 是 transport，不决定状态。Coordinator 从 Git 重新计算 SHA、diff、paths 和 overlap；claimed values 只是输入，不能作为验证证据。

```go
type ProposalReceipt struct {
    ReceiptID        string    `json:"receipt_id"`
    ProposalID       string    `json:"proposal_id"`
    ProposalDigest   string    `json:"proposal_digest"`
    AssignmentDigest string    `json:"assignment_digest"`
    WorkflowDigest   string    `json:"workflow_digest"`
    FencingEpoch     uint64    `json:"fencing_epoch"`
    Result           string    `json:"result"` // accepted|rejected|expired
    Reasons          []string  `json:"reasons,omitempty"`
    RecordedAt       time.Time `json:"recorded_at"`
    Digest           string    `json:"digest"`
}
```

Coordinator 先把完整 Proposal 写成 immutable `ProposalReceived` record，再写 Receipt；两者进入 append-only Coordination namespace。相同 ProposalID+ProposalDigest 重放幂等；相同 ID 不同 digest 返回冲突。Inbox envelope 只有在完整 Proposal record 和 durable receipt 都落盘后才能清理。

## Evidence

```go
type Evidence struct {
    EvidenceID       string        `json:"evidence_id"`
    Kind             string        `json:"kind"`
    Trust            EvidenceTrust `json:"trust"`
    ProofScopes      []ProofScope  `json:"proof_scopes"`
    Actor            ActorRef      `json:"actor"`
    WorkRef          WorkRef       `json:"work_ref"`
    WorkflowDigest   string        `json:"workflow_digest"`
    SkillPlanDigest  string        `json:"skill_plan_digest"`
    GitHead          string        `json:"git_head,omitempty"`
    TreeDigest       string        `json:"tree_digest,omitempty"`
    DirtyTreeDigest  string        `json:"dirty_tree_digest,omitempty"`
    ChangedPaths     []string      `json:"changed_paths,omitempty"`
    StartedAt        time.Time     `json:"started_at,omitempty"`
    FinishedAt       time.Time     `json:"finished_at,omitempty"`
    DurationMS       int64         `json:"duration_ms,omitempty"`
    Argv             []string      `json:"argv,omitempty"`
    Cwd              string        `json:"cwd,omitempty"`
    ExitCode         *int          `json:"exit_code,omitempty"`
    Signal           string        `json:"signal,omitempty"`
    Environment      EnvironmentSummary `json:"environment,omitempty"`
    Stdout           ContentRef    `json:"stdout,omitempty"`
    Stderr           ContentRef    `json:"stderr,omitempty"`
    Artifacts        []ContentRef  `json:"artifacts,omitempty"`
    ManifestDigest   string        `json:"manifest_digest"`
}
```

Trust order：`observed_process > ci_attestation > file_digest > external_reference > manual_attestation`。

ProofScope：`static|unit|integration|e2e|production_wiring|external_side_effect`。两者不能压缩成一个“confidence”。

## Execution

```go
type Execution struct {
    ExecutionID      string      `json:"execution_id"`
    WorkRef          WorkRef     `json:"work_ref"`
    WorkflowDigest   string      `json:"workflow_digest"`
    SkillPlanDigest  string      `json:"skill_plan_digest"`
    Actor             ActorRef    `json:"actor"`
    Contributors      []ActorRef  `json:"contributors"`
    BaseCommit        string      `json:"base_commit"`
    HeadCommit        string      `json:"head_commit"`
    TreeDigest        string      `json:"tree_digest"`
    Deliverables      []ObjectRef `json:"deliverables"`
    EvidenceRefs      []string    `json:"evidence_refs"`
    EvidenceSetDigest string      `json:"evidence_set_digest"`
    KnownGaps         []string    `json:"known_gaps"`
    ResidualRisks     []string    `json:"residual_risks"`
    SubmittedAt       time.Time   `json:"submitted_at"`
}
```

## Review

```go
type Review struct {
    ReviewID          string        `json:"review_id"`
    ExecutionID       string        `json:"execution_id"`
    WorkRef           WorkRef       `json:"work_ref"`
    WorkflowDigest    string        `json:"workflow_digest"`
    SkillPlanDigest   string        `json:"skill_plan_digest"`
    TreeDigest        string        `json:"tree_digest"`
    EvidenceSetDigest string        `json:"evidence_set_digest"`
    Reviewer          ActorRef      `json:"reviewer"`
    Independence      string        `json:"independence"` // strong|medium|weak
    Verdict           string        `json:"verdict"` // accepted|rejected|limited
    Findings          []Finding     `json:"findings"`
    SubmittedAt       time.Time     `json:"submitted_at"`
}
```

高风险 Gate 要求 Reviewer 不在 Execution contributors 中，且 Session 不同。早期身份是 provenance 约束，不宣传为密码学认证。

## GateReceipt

```go
type GateReceipt struct {
    GateReceiptID    string       `json:"gate_receipt_id"`
    OperationID      string       `json:"operation_id"`
    WorkRef          WorkRef      `json:"work_ref"`
    SkillPlanDigest  string       `json:"skill_plan_digest"`
    RequiredGateSetDigest string   `json:"required_gate_set_digest"`
    GatePolicyDigest string       `json:"gate_policy_digest"`
    ContractApprovalDigest string `json:"contract_approval_digest"`
    ExpectedWorkflowDigest string `json:"expected_workflow_digest"`
    ProposedSuccessorDigest string `json:"proposed_successor_digest"`
    FencingEpoch     uint64       `json:"fencing_epoch"`
    ExecutionID      string       `json:"execution_id"`
    ReviewID         string       `json:"review_id,omitempty"`
    TreeDigest       string       `json:"tree_digest"`
    EvidenceSetDigest string      `json:"evidence_set_digest"`
    CriterionCoverage []CriterionCoverage `json:"criterion_coverage"`
    Result           string       `json:"result"` // verified|limited|failed
    Gaps             []string     `json:"gaps"`
    ResidualRisks    []string     `json:"residual_risks"`
    EvaluatedAt      time.Time    `json:"evaluated_at"`
    Digest           string       `json:"digest"`
}
```

GateReceipt immutable，但只表达 evaluation。`RequiredGateSetDigest` 对 SkillPlan 中规范化后的 required GateSpec 集合计算；`GatePolicyDigest` 绑定实际用于 evaluation 的 Policy 版本。任一 SkillPlan/GateSpec/Policy digest 改变都必须重新评估，不能让旧 Receipt 被新 Policy 重新解释。`Result=failed` 永不生成完成权限；`Result=limited` 只有对应 GateSpec `LimitedAllowed=true` 且有 trusted Host user-interaction UserAcceptanceReceipt 时才可能授权。

```go
type UserAcceptanceReceipt struct {
    AcceptanceID    string    `json:"acceptance_id"`
    User            ActorRef  `json:"user"` // provenance only; role=user is not authorization
    WorkRef         WorkRef   `json:"work_ref"`
    GateReceiptDigest string  `json:"gate_receipt_digest"`
    GatePolicyDigest string   `json:"gate_policy_digest"`
    InteractionReceiptDigest string `json:"interaction_receipt_digest"`
    AcceptedGaps    []string  `json:"accepted_gaps"`
    AcceptedRisks   []string  `json:"accepted_risks"`
    AcceptedAt      time.Time `json:"accepted_at"`
    Digest          string    `json:"digest"`
}

type CompletionAuthorization struct {
    AuthorizationID  string    `json:"authorization_id"`
    OperationID      string    `json:"operation_id"`
    WorkRef          WorkRef   `json:"work_ref"`
    SkillPlanDigest  string    `json:"skill_plan_digest"`
    RequiredGateSetDigest string `json:"required_gate_set_digest"`
    GateReceiptDigest string   `json:"gate_receipt_digest"`
    GatePolicyDigest string    `json:"gate_policy_digest"`
    ContractApprovalDigest string `json:"contract_approval_digest"`
    UserAcceptanceDigest string `json:"user_acceptance_digest,omitempty"`
    ExpectedWorkflowDigest string `json:"expected_workflow_digest"`
    AuthorizedSuccessorDigest string `json:"authorized_successor_digest"`
    TreeDigest       string    `json:"tree_digest"`
    EvidenceSetDigest string   `json:"evidence_set_digest"`
    FencingEpoch    uint64    `json:"fencing_epoch"`
    IssuedAt        time.Time `json:"issued_at"`
    Digest          string    `json:"digest"`
}

type CancellationAuthorization struct {
    AuthorizationID  string    `json:"authorization_id"`
    WorkRef           WorkRef   `json:"work_ref"`
    ExpectedWorkflowDigest string `json:"expected_workflow_digest"`
    AuthorizedSuccessorDigest string `json:"authorized_successor_digest"`
    CancellationPolicyDigest string `json:"cancellation_policy_digest"`
    InteractionReceiptDigest string `json:"interaction_receipt_digest"`
    FencingEpoch      uint64    `json:"fencing_epoch"`
    IssuedAt          time.Time `json:"issued_at"`
    Digest            string    `json:"digest"`
}
```

所有授权先生成通用 immutable `TrustedUserInteractionReceipt`。Engine 发出一次性 request/challenge；只有 Host 提供的、模型工具能力无法伪造的明确用户交互通道才能返回 proof。Receipt 固定 `purpose`、scope、ProjectID/WorkIdentity、target/request digest、interaction ID 和 Host receipt；Evolution second confirmation 必须使用不同 `InteractionID`。`scope=work` 必须同时带匹配的 ProjectID/WorkIdentity，`scope=project` 必须带 ProjectID，`scope=global` 禁止带 WorkIdentity。若 Host 不能区分用户动作与 Agent 工具调用，则返回 `USER_ACCEPTANCE_UNAVAILABLE`，不得降级为 `ActorRef.Role=user`。

`UserAcceptanceReceipt` 不能由模型、Worker 或 Coordinator 构造。Engine 验证被引用的 trusted interaction purpose=`limited-acceptance`、GateReceipt/Policy target、accepted gaps/risks 后才可追加；Coordinator 只是请求者，不是正式 Writer 或用户授权者。Cancellation、Evolution approval/confirm 和 Policy approval 同样只能引用用途、target 和 challenge 都匹配的 trusted receipt。

Authorization generator 必须读取 GateReceipt：`verified` 可按 Gate Policy 生成；`limited` 需要 `LimitedAllowed=true` 和同 WorkRef/GateReceipt/Policy/gaps/risks 的 UserAcceptanceReceipt；`failed` 必须拒绝。Authorization 的 `SkillPlanDigest`、`RequiredGateSetDigest`、`GatePolicyDigest` 和 `ContractApprovalDigest` 必须逐字等于 GateReceipt；任一变化都要重评 Gate。消费前，previous/successor/fencing/tree/evidence-set、source digest 或 Gate contract/approval 改变即 stale。

Cancellation 不冒充“完成”。Engine 只有在 trusted Host 明确用户交互和 cancellation Policy 都匹配时才生成 `CancellationAuthorization`；它只允许一次 exact active/blocked/in_review→cancelled successor，不要求完成 Gate，也不能满足 Completion Claim。

## Delivery Coverage Matrix

```go
type CriterionCoverage struct {
    CriterionID        string       `json:"criterion_id"`
    WorkRef            WorkRef      `json:"work_ref"`
    Owner              ActorRef     `json:"owner,omitempty"`
    Implementation     string       `json:"implementation"`
    EvidenceTrust      EvidenceTrust `json:"evidence_trust,omitempty"`
    ProofScopes        []ProofScope `json:"proof_scopes,omitempty"`
    ReviewStatus       string       `json:"review_status"`
    ProductionWiring   string       `json:"production_wiring"`
    GitProposalState   string       `json:"git_proposal_state"`
    Result             string       `json:"result"` // verified|limited|failed
    Gaps               []string     `json:"gaps,omitempty"`
    ResidualRisks      []string     `json:"residual_risks,omitempty"`
}
```

CSV/GUI 由这些 typed records 派生，不能反向覆盖。

## WorkflowCheckpoint

```go
type WorkflowCheckpoint struct {
    CheckpointID       string    `json:"checkpoint_id"`
    OperationID        string    `json:"operation_id"`
    AdapterVersion     string    `json:"adapter_version"`
    PreviousDigest     string    `json:"previous_digest"`
    NewDigest          string    `json:"new_digest"`
    CompletionAuthorizationDigest string `json:"completion_authorization_digest,omitempty"`
    ChangedPaths       []string  `json:"changed_paths"`
    Coordinator        ActorRef  `json:"coordinator"`
    FencingEpoch       uint64    `json:"fencing_epoch"`
    AcceptedAt         time.Time `json:"accepted_at"`
}
```

Pending marker 不是 Authority。崩溃恢复只有在 marker、current fencing epoch、expected digest 和唯一合法后继匹配时才能追加 checkpoint。完成 transition 的 checkpoint 还必须加载 CompletionAuthorization，验证其 Gate result/policy/User acceptance，证明 `PreviousDigest == Authorization.ExpectedWorkflowDigest`、`NewDigest == Authorization.AuthorizedSuccessorDigest`，并把 `CompletionAuthorizationDigest` 写入 Workflow 完成记录或可验证 reference。

## Lite -> GSD Promotion Records

```go
type PromotionPlan struct {
    PromotionID       string `json:"promotion_id"`
    ProjectID         string `json:"project_id"`
    ExpectedLiteRevision uint64 `json:"expected_lite_revision"`
    ExpectedLiteWorkingSetDigest string `json:"expected_lite_working_set_digest"`
    LiteHandoffBackup ExactBackupRef `json:"lite_handoff_backup"`
    GSDPreimageState  string `json:"gsd_preimage_state"` // absent|present_empty
    GSDPreimageDigest string `json:"gsd_preimage_digest,omitempty"`
    GSDPreimageBackup *ExactBackupRef `json:"gsd_preimage_backup,omitempty"`
    StagedGSDDigest   string `json:"staged_gsd_digest"`
    SuccessorHandoffDigest string `json:"successor_handoff_digest"`
    FencingEpoch     uint64 `json:"fencing_epoch"`
    Digest           string `json:"digest"`
}

type PromotionFence struct {
    PromotionID      string `json:"promotion_id"`
    ExpectedLiteWorkingSetDigest string `json:"expected_lite_working_set_digest"`
    GSDPreimageDigest string `json:"gsd_preimage_digest,omitempty"`
    FencingEpoch     uint64 `json:"fencing_epoch"`
    State            string `json:"state"` // active|committed|rolled_back
    Digest           string `json:"digest"`
}

type PromotionPending struct {
    PromotionID      string `json:"promotion_id"`
    ExpectedLiteWorkingSetDigest string `json:"expected_lite_working_set_digest"`
    GSDPreimageDigest string `json:"gsd_preimage_digest,omitempty"`
    StagedGSDDigest  string `json:"staged_gsd_digest"`
    SuccessorHandoffDigest string `json:"successor_handoff_digest"`
    FencingEpoch     uint64 `json:"fencing_epoch"`
    Digest           string `json:"digest"`
}

type LitePromoted struct {
    PromotionID      string    `json:"promotion_id"`
    PreviousLiteWorkingSetDigest string `json:"previous_lite_working_set_digest"`
    GSDWorkflowDigest string   `json:"gsd_workflow_digest"`
    GSDHandoffDigest string    `json:"gsd_handoff_digest"`
    FencingEpoch     uint64    `json:"fencing_epoch"`
    PromotedAt       time.Time `json:"promoted_at"`
    Digest           string    `json:"digest"`
}

type PromotionRolledBack struct {
    PromotionID      string    `json:"promotion_id"`
    RestoredLiteHandoffDigest string `json:"restored_lite_handoff_digest"`
    RestoredGSDPreimageDigest string `json:"restored_gsd_preimage_digest,omitempty"`
    FencingEpoch     uint64    `json:"fencing_epoch"`
    RolledBackAt     time.Time `json:"rolled_back_at"`
    Digest           string    `json:"digest"`
}
```

这些 record 位于 append-only Promotion Control namespace，由 Engine 写入；它们不拥有 GSD Phase/Task，只证明一次 backend cutover 的 expected preimage、唯一 successor 与恢复状态。Pending/fence/receipt digest 不匹配、未知 control suffix 或同时存在两个合法 successor 时 fail-closed。

## Migration Records

```go
type MigrationPlan struct {
    MigrationID     string   `json:"migration_id"`
    SourceMode      string   `json:"source_mode"` // native-v2
    TargetMode      string   `json:"target_mode"` // lite|gsd
    SourceHead      string   `json:"source_head"`
    SourceDigest    string   `json:"source_digest"`
    SourceBackupRef ExactBackupRef `json:"source_backup_ref"`
    TargetPreimageState string `json:"target_preimage_state"` // absent|present_empty|adopted
    TargetPreimageDigest string `json:"target_preimage_digest,omitempty"`
    TargetPreimageBackupRef *ExactBackupRef `json:"target_preimage_backup_ref,omitempty"`
    SemanticSummary string   `json:"semantic_summary"`
    Mapping         []MigrationMapping `json:"mapping"`
    Risks           []string `json:"risks"`
}

type MigrationReceipt struct {
    MigrationID      string `json:"migration_id"`
    SourceBackupDigest string `json:"source_backup_digest"`
    TargetPreimageDigest string `json:"target_preimage_digest,omitempty"`
    TargetPreimageBackupDigest string `json:"target_preimage_backup_digest,omitempty"`
    TargetDigest     string `json:"target_digest"`
    HandoffDigest    string `json:"handoff_digest"`
    FenceDigest      string `json:"fence_digest"`
    TombstoneDigest  string `json:"tombstone_digest"`
    InitialBusinessTrustDigest string `json:"initial_business_trust_digest"`
    InitialCoordinationDigest string `json:"initial_coordination_digest"`
    InitialProjectContractDigest string `json:"initial_project_contract_digest"`
}
```

```go
type MigrationFence struct {
    MigrationID          string `json:"migration_id"`
    SourceHead           string `json:"source_head"`
    TargetPreimageDigest string `json:"target_preimage_digest,omitempty"`
    FencingEpoch         uint64 `json:"fencing_epoch"`
    State                string `json:"state"` // active|committed|rolled_back
    Digest               string `json:"digest"`
}

type MigrationRollbackClosed struct {
    MigrationID       string    `json:"migration_id"`
    FirstWriteKind    string    `json:"first_write_kind"` // lite|gsd|trust|coordination|project-contract
    FirstWriteDigest  string    `json:"first_write_digest"`
    TargetDigest      string    `json:"target_digest"`
    HandoffDigest     string    `json:"handoff_digest"`
    PreviousBusinessTrustDigest string `json:"previous_business_trust_digest"`
    PreviousCoordinationDigest string `json:"previous_coordination_digest"`
    PreviousProjectContractDigest string `json:"previous_project_contract_digest"`
    FencingEpoch      uint64    `json:"fencing_epoch"`
    ClosedAt          time.Time `json:"closed_at"`
    Digest            string    `json:"digest"`
}
```

`ExactBackupRef` 的 manifest 必须逐项绑定 path kind、mode、uid/gid（平台可用时）、mtime policy、symlink target 和 content digest；`OriginalBytes == StoredBytes`、无截断、无脱敏，并使用 durable retention。`present_empty` 仍需保存目录 metadata；只有 `absent` 才允许 rollback 删除目标路径。

MigrationPlan/Fence/Receipt/RollbackClosed 等控制记录位于独立 append-only **Migration Control namespace**，不进入 Business Trust/Coordination/Project Contract digest。`BusinessTrustDigest(project)` 只投影 Evidence、Execution、Review、GateReceipt、UserAcceptance、CompletionAuthorization、CancellationAuthorization，以及 `scope=project|work` 且 ProjectID 匹配的 TrustedUserInteractionReceipt；global interaction 明确排除。`CoordinationDigest(project)` 投影该 Project 的 ActivitySkillPlan/Assignment/Proposal/receipts；`ProjectContractDigest(project)` 只投影绑定该 Project/WorkIdentity 的 SkillPlanContract/candidate/approval。全局、未绑定 Project/WorkIdentity 的 SkillManifest/Gate/Policy Contract 可以排除。Migration receipt/close/rollback 必须调用同一 project projection 函数。Control namespace 每个 record 仍有 previous/digest chain；rollback 还要求其后缀恰好符合当前 migration state machine，不能忽略未知控制记录。

Dry-run 不能写任何项目文件。Apply 必须在 target/Handoff write 前、持有 Git common-dir source commit lock 和目标 Coordinator lease 时 durable-install `State=active` fence；legacy Writer 不认识、不共享 lock 或不在最终 commit 前重验 fence时 migration fail-closed。Active fence 阻止所有 legacy write，也阻止非 migration-recovery 的 Lite/GSD/Business Trust/Coordination/project-bound Contract Authority write。Committed tombstone 永久阻止 legacy writer；上述任一 namespace 的首个 v3 Authority write 都必须先写 pending marker，再在触碰 Authority 前向 Migration Control namespace durable-append `MigrationRollbackClosed`，绑定 intended exact successor/business record digest和三个 previous digest，之后才允许提交首写。Rollback 仅在不存在该 record，且 target、Handoff、Business Trust、Coordination、Project Contract digest仍精确等于 MigrationReceipt 初始摘要，且 Control suffix 合法时可执行；恢复 source、target preimage和 Handoff 后最后关闭 fence。

`MigrationRollbackClosed.FirstWriteDigest` 指向 pending operation 的业务 record 或 authorized Workflow successor digest，不包含 close record自身；三个 `Previous*Digest` 必须分别等于 MigrationReceipt 的 initial digest，也不包含任何 migration control record。Engine 必须先验证 intended digest 可由唯一合法后继产生；close durable 后即使业务 commit 尚未完成也禁止 rollback，recovery 只可完成/reconcile。对 Trust/Coordination/project-contract first write，close record 与至少一个对应 namespace 的独立业务 record属于同一 first-write operation。这样关闭 rollback 不形成摘要自引用，也不能用一条空 close record伪造“已经开始 v3 工作”。

## Projection Contract

每个 Projection 保存：

```text
schema_version
authority_kind
authority_ref
source_revision
source_digest
projector_version
built_at
content_digest
```

不匹配即 stale；可以 refresh/rebuild，不能反向写 Authority。

## Legacy v2 Mapping

| v2 类型 | v3 处理 |
|---|---|
| Root Objective | legacy source；迁移到 Lite working set 或 GSD Requirement/Phase context |
| WorkItem | legacy source；若迁移 GSD则映射为 GSD Plan/Task，不在 Trust Journal 复制状态 |
| Objective-bound Evidence | 转为 WorkRef + source/workflow digest bound Evidence |
| v2 transaction chain | 保留原字节和摘要，未来作为 Trust/migration audit source |
| v2 Handoff `mode=native` | `legacy-native` 读取；新 write 返回 `MIGRATION_REQUIRED` |
| v2 Snapshot/Projection | 可删除；不能定义 v3 Authority |
