# Summer Harness v2 Data Model

本文件描述 v2 实现契约。`CONTEXT.md` 定义领域语言；这里定义可序列化形状、约束和状态转换。

## Command Envelope

```go
type CommandEnvelope struct {
    Schema           string          `json:"schema"`            // summer.command/v2
    CommandID        string          `json:"command_id"`        // ULID
    IdempotencyKey   string          `json:"idempotency_key"`
    CorrelationID    string          `json:"correlation_id"`
    CausationID      string          `json:"causation_id,omitempty"`
    ProjectID        string          `json:"project_id"`
    ExpectedRevision uint64          `json:"expected_revision"`
    Actor            ActorRef        `json:"actor"`
    IssuedAt         time.Time       `json:"issued_at"`
    Kind             CommandKind     `json:"kind"`
    Payload          json.RawMessage `json:"payload"`
}
```

同一 `ProjectID + IdempotencyKey` 只能产生一个 committed receipt。`ExpectedRevision` 不匹配时拒绝，不自动覆盖。

## Actor

```go
type ActorRef struct {
    ActorID   string    `json:"actor_id"`
    SessionID string    `json:"session_id"`
    Runtime   string    `json:"runtime"`
    Model     string    `json:"model,omitempty"`
    Role      ActorRole `json:"role"` // user|coordinator|worker|reviewer|system
}
```

ActorRef 是 provenance，不是密码学身份。User approval 需要本地交互确认或未来显式签名 Adapter。

## Root Objective

```go
type RootObjective struct {
    ObjectiveID   string          `json:"objective_id"`
    Title         string          `json:"title"`
    Goal          string          `json:"goal"`
    Acceptance    []Criterion     `json:"acceptance"`
    Status        ObjectiveStatus `json:"status"`
    Profile       RiskProfile     `json:"profile"`
    Revision      uint64          `json:"revision"`
    FocusWorkItem string          `json:"focus_work_item,omitempty"`
    ResidualRisks []string        `json:"residual_risks"`
}
```

状态：

```text
active <-> blocked
active/blocked -> review -> completed
review -> active
active/blocked/review -> cancelled
```

`completed` 必须通过当前 revision 的 Gate；不能直接修改状态字段。

## WorkItem

```go
type WorkItem struct {
    WorkItemID  string         `json:"work_item_id"`
    ObjectiveID string         `json:"objective_id"`
    Title       string         `json:"title"`
    Outcome     string         `json:"outcome"`
    Acceptance  []Criterion    `json:"acceptance"`
    Status      WorkItemStatus `json:"status"`
    Revision    uint64         `json:"revision"`
    DependsOn   []string       `json:"depends_on"`
    Assignment  string         `json:"assignment_id,omitempty"`
}
```

`depends_on` 必须无环。一个 active WorkItem 最多有一个非终态 Assignment。

## Assignment

```go
type Assignment struct {
    AssignmentID  string    `json:"assignment_id"`
    WorkItemID    string    `json:"work_item_id"`
    Owner         ActorRef  `json:"owner"`
    BaseCommit    string    `json:"base_commit"`
    Branch        string    `json:"branch"`
    Worktree      string    `json:"worktree"`
    AllowedPaths  []string  `json:"allowed_paths"`
    Acceptance    []string  `json:"acceptance"`
    CapabilityHash string   `json:"capability_hash"`
    LeaseExpiresAt time.Time `json:"lease_expires_at,omitempty"`
    Status        string    `json:"status"` // assigned|active|submitted|released|expired
}
```

路径必须是 repo-relative、不能包含 `..`、不能经过符号链接。Capability 只防误提交和串单，不作为恶意本机进程隔离。

## Worker Proposal

```go
type Proposal struct {
    ProposalID    string        `json:"proposal_id"`
    AssignmentID  string        `json:"assignment_id"`
    Actor         ActorRef      `json:"actor"`
    BaseCommit    string        `json:"base_commit"`
    HeadCommit    string        `json:"head_commit"`
    ChangedPaths  []string      `json:"changed_paths"`
    DiffDigest    string        `json:"diff_digest"`
    EvidenceRefs  []string      `json:"evidence_refs"`
    Deliverables  []ObjectRef   `json:"deliverables"`
    KnownGaps     []string      `json:"known_gaps"`
    ResidualRisks []string      `json:"residual_risks"`
    SubmittedAt   time.Time     `json:"submitted_at"`
}
```

Proposal 创建后不可修改。Coordinator ingest 时必须重新计算 branch、diff 和 allowed path，不信任 Worker 自报。

## Evidence

```go
type Evidence struct {
    EvidenceID       string        `json:"evidence_id"`
    Kind             EvidenceKind  `json:"kind"`
    Trust            EvidenceTrust `json:"trust"`
    Actor             ActorRef      `json:"actor"`
    WorkItemID        string        `json:"work_item_id,omitempty"`
    CoveredRevision   uint64        `json:"covered_revision"`
    GitHead           string        `json:"git_head,omitempty"`
    DirtyTreeDigest   string        `json:"dirty_tree_digest,omitempty"`
    StartedAt         time.Time     `json:"started_at,omitempty"`
    FinishedAt        time.Time     `json:"finished_at,omitempty"`
    DurationMS        int64         `json:"duration_ms,omitempty"`
    Argv              []string      `json:"argv,omitempty"`
    Cwd               string        `json:"cwd,omitempty"`
    ExitCode          *int          `json:"exit_code,omitempty"`
    Signal            string        `json:"signal,omitempty"`
    Stdout             ContentRef    `json:"stdout,omitempty"`
    Stderr             ContentRef    `json:"stderr,omitempty"`
    Artifacts          []ContentRef  `json:"artifacts,omitempty"`
    ExternalReference string        `json:"external_reference,omitempty"`
}
```

Evidence 创建后不可修改。新的运行产生新的 Evidence；失效通过关联事件记录。

## Execution

```go
type Execution struct {
    ExecutionID      string      `json:"execution_id"`
    WorkItemID       string      `json:"work_item_id"`
    WorkItemRevision uint64      `json:"work_item_revision"`
    Actor            ActorRef    `json:"actor"`
    BaseCommit       string      `json:"base_commit"`
    HeadCommit       string      `json:"head_commit"`
    TreeDigest       string      `json:"tree_digest"`
    Deliverables     []ObjectRef `json:"deliverables"`
    EvidenceRefs     []string    `json:"evidence_refs"`
    EvidenceSetDigest string     `json:"evidence_set_digest"`
    KnownGaps        []string    `json:"known_gaps"`
    ResidualRisks    []string    `json:"residual_risks"`
    SubmittedAt      time.Time   `json:"submitted_at"`
}
```

Execution 创建后不可修改。变更代码、WorkItem revision 或 Evidence 集合都必须提交新 Execution。

## Review

```go
type Review struct {
    ReviewID          string        `json:"review_id"`
    ExecutionID       string        `json:"execution_id"`
    WorkItemRevision  uint64        `json:"work_item_revision"`
    TreeDigest        string        `json:"tree_digest"`
    EvidenceSetDigest string        `json:"evidence_set_digest"`
    Reviewer          ActorRef      `json:"reviewer"`
    Verdict           ReviewVerdict `json:"verdict"`
    Findings          []Finding     `json:"findings"`
    SubmittedAt       time.Time     `json:"submitted_at"`
}
```

高风险 Gate 要求 Reviewer 不是 Execution contributor，且不能复用执行 Session。

## Evolution Candidate

```go
type EvolutionCandidate struct {
    CandidateID    string          `json:"candidate_id"`
    Pattern        string          `json:"pattern"`
    SourceRefs     []string        `json:"source_refs"`
    Occurrences    int             `json:"occurrences"`
    Confidence     float64         `json:"confidence"`
    CounterExamples []string       `json:"counter_examples"`
    ProposedKind   string          `json:"proposed_kind"`
    ProposedPatch  ContentRef      `json:"proposed_patch"`
    Scope          string          `json:"scope"` // project|user
    Risk           string          `json:"risk"`
    Status         CandidateStatus `json:"status"`
    ValidationPlan []string        `json:"validation_plan"`
    RollbackPlan   []string        `json:"rollback_plan"`
}
```

状态：`candidate -> approved|rejected -> applied -> verified|rolled_back`。只有 User Actor 能批准。

## Committed Transaction

```go
type TransactionManifest struct {
    TransactionID string      `json:"transaction_id"`
    PreviousDigest string     `json:"previous_digest"`
    Revision      uint64      `json:"revision"`
    Command       CommandRef  `json:"command"`
    Actor         ActorRef    `json:"actor"`
    EventFiles    []ObjectRef `json:"event_files"`
    EventsDigest  string      `json:"events_digest"`
    CommittedAt   time.Time   `json:"committed_at"`
}
```

Manifest、事件集合和前驱摘要共同形成 transaction digest。只有被 `HEAD` 引用且摘要链连续的 transaction 参与状态 fold。

## Projection Contract

每个 Projection 必须记录：

```text
schema_version, project_id, ledger_head, ledger_revision,
projector_version, built_at, content_digest
```

任何字段不匹配都视为 stale；Projection 可以刷新或重建，但不能反向写 Canonical Ledger。
