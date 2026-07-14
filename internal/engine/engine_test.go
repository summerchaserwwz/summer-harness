package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

func TestStartObjectiveIsReadableThroughEngine(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	kernel := engine.New(store)
	payload, err := json.Marshal(engine.StartObjective{
		Title:      "构建 Evidence capture",
		Goal:       "验证结果不能由 Agent 手写伪造",
		Acceptance: []string{"测试命令产生 machine-captured receipt"},
		Profile:    "high-risk",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}

	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-start-1",
		IdempotencyKey:   "start-objective",
		CorrelationID:    "corr-1",
		IssuedAt:         time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC),
		ProjectID:        "project-summer-harness",
		ExpectedRevision: 0,
		Actor: engine.ActorRef{
			ActorID:   "user-summer",
			SessionID: "session-1",
			Runtime:   "codex",
			Role:      engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("apply StartObjective: %v", err)
	}
	if !receipt.Accepted {
		t.Fatalf("receipt rejected: %#v", receipt.Rejection)
	}
	if receipt.NewRevision != 1 {
		t.Fatalf("new revision = %d, want 1", receipt.NewRevision)
	}
	if receipt.EntityID == "" {
		t.Fatal("receipt has no objective entity id")
	}

	view, err := kernel.Query(context.Background(), engine.Query{
		Kind:      engine.QueryObjective,
		ProjectID: "project-summer-harness",
		EntityID:  receipt.EntityID,
	})
	if err != nil {
		t.Fatalf("query objective: %v", err)
	}
	objectiveView, ok := view.(engine.ObjectiveView)
	if !ok {
		t.Fatalf("view type = %T, want engine.ObjectiveView", view)
	}
	objective := objectiveView.Objective
	if objective.Status != engine.ObjectiveActive {
		t.Fatalf("status = %q, want %q", objective.Status, engine.ObjectiveActive)
	}
	if objective.Revision != 1 {
		t.Fatalf("objective revision = %d, want 1", objective.Revision)
	}
	if objective.Goal != "验证结果不能由 Agent 手写伪造" {
		t.Fatalf("goal = %q", objective.Goal)
	}
	if len(objective.Acceptance) != 1 || objective.Acceptance[0] != "测试命令产生 machine-captured receipt" {
		t.Fatalf("acceptance = %#v", objective.Acceptance)
	}
}

func TestStartObjectiveRetryReturnsOriginalReceipt(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	kernel := engine.New(store)
	payload, err := json.Marshal(engine.StartObjective{
		Title:      "构建 transaction ledger",
		Goal:       "命令重试不产生重复状态",
		Acceptance: []string{"相同 idempotency key 返回同一 receipt"},
		Profile:    "standard",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}
	command := engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-retry-1",
		IdempotencyKey:   "start-once",
		CorrelationID:    "corr-retry",
		IssuedAt:         time.Date(2026, 7, 15, 9, 1, 0, 0, time.UTC),
		ProjectID:        "project-retry",
		ExpectedRevision: 0,
		Actor: engine.ActorRef{
			ActorID:   "user-summer",
			SessionID: "session-retry",
			Runtime:   "codex",
			Role:      engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: payload,
	}

	first, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if !second.Accepted {
		t.Fatalf("retry rejected: %#v", second.Rejection)
	}
	if second.TransactionID != first.TransactionID || second.EntityID != first.EntityID || second.NewRevision != first.NewRevision {
		t.Fatalf("retry receipt = %#v, want original %#v", second, first)
	}
}

func TestIdempotencyKeyCannotBeReusedForDifferentCommand(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	kernel := engine.New(store)
	firstPayload, err := json.Marshal(engine.StartObjective{
		Title:      "第一个目标",
		Goal:       "只能提交一次",
		Acceptance: []string{"产生一个 objective"},
		Profile:    "standard",
	})
	if err != nil {
		t.Fatalf("marshal first payload: %v", err)
	}
	secondPayload, err := json.Marshal(engine.StartObjective{
		Title:      "不同目标",
		Goal:       "不应复用旧 receipt",
		Acceptance: []string{"返回 idempotency conflict"},
		Profile:    "standard",
	})
	if err != nil {
		t.Fatalf("marshal second payload: %v", err)
	}
	base := engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-idempotency-first",
		IdempotencyKey:   "shared-key",
		CorrelationID:    "corr-idempotency",
		IssuedAt:         time.Date(2026, 7, 15, 9, 2, 0, 0, time.UTC),
		ProjectID:        "project-idempotency-conflict",
		ExpectedRevision: 0,
		Actor: engine.ActorRef{
			ActorID:   "user-summer",
			SessionID: "session-idempotency",
			Runtime:   "codex",
			Role:      engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: firstPayload,
	}
	if receipt, err := kernel.Apply(context.Background(), base); err != nil || !receipt.Accepted {
		t.Fatalf("first apply receipt=%#v err=%v", receipt, err)
	}

	conflict := base
	conflict.CommandID = "cmd-idempotency-second"
	conflict.Payload = secondPayload
	receipt, err := kernel.Apply(context.Background(), conflict)
	if err != nil {
		t.Fatalf("conflicting retry returned error: %v", err)
	}
	if receipt.Accepted {
		t.Fatalf("conflicting retry accepted: %#v", receipt)
	}
	if receipt.Rejection == nil || receipt.Rejection.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("rejection = %#v, want IDEMPOTENCY_CONFLICT", receipt.Rejection)
	}
}

func TestIdempotencyDigestPreservesLargeJSONNumbers(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "large-number", 0)
	command.Payload = json.RawMessage(`{"title":"目标","goal":"区分大整数","acceptance":["返回冲突"],"profile":"standard","nonce":9007199254740992}`)
	first, err := kernel.Apply(context.Background(), command)
	if err != nil || !first.Accepted {
		t.Fatalf("first large-number command receipt=%#v err=%v", first, err)
	}
	command.Payload = json.RawMessage(`{"title":"目标","goal":"区分大整数","acceptance":["返回冲突"],"profile":"standard","nonce":9007199254740993}`)
	second, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("second large-number command returned error: %v", err)
	}
	if second.Accepted || second.Rejection == nil || second.Rejection.Code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("second large-number receipt = %#v, want IDEMPOTENCY_CONFLICT", second)
	}
}

func TestObjectiveSurvivesEngineRestartWithFileLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	firstStore, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new first file ledger: %v", err)
	}
	firstEngine := engine.New(firstStore)
	payload, err := json.Marshal(engine.StartObjective{
		Title:      "跨 Session 恢复",
		Goal:       "新的 Engine 能恢复同一 Root Objective",
		Acceptance: []string{"独立实例读到 revision 1"},
		Profile:    "high-risk",
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}
	receipt, err := firstEngine.Apply(context.Background(), engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-persist-1",
		IdempotencyKey:   "persist-objective",
		CorrelationID:    "corr-persist",
		IssuedAt:         time.Date(2026, 7, 15, 9, 3, 0, 0, time.UTC),
		ProjectID:        "project-persist",
		ExpectedRevision: 0,
		Actor: engine.ActorRef{
			ActorID:   "user-summer",
			SessionID: "session-before-restart",
			Runtime:   "codex",
			Role:      engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: payload,
	})
	if err != nil || !receipt.Accepted {
		t.Fatalf("apply before restart receipt=%#v err=%v", receipt, err)
	}

	secondStore, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new second file ledger: %v", err)
	}
	secondEngine := engine.New(secondStore)
	view, err := secondEngine.Query(context.Background(), engine.Query{
		Kind:      engine.QueryObjective,
		ProjectID: "project-persist",
		EntityID:  receipt.EntityID,
	})
	if err != nil {
		t.Fatalf("query after restart: %v", err)
	}
	objectiveView, ok := view.(engine.ObjectiveView)
	if !ok || objectiveView.Objective.Revision != 1 || objectiveView.Objective.Goal != "新的 Engine 能恢复同一 Root Objective" {
		t.Fatalf("objective after restart = %#v", view)
	}
}

func TestResumeUsesTheVerifiedSnapshotAfterValidatingTheLedgerChain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatalf("new continuity module: %v", err)
	}
	store := &transactionCountingStore{Store: ledger.NewMemory()}
	kernel := engine.New(store, engine.WithContinuity(module))
	receipt, err := kernel.Apply(context.Background(), startObjectiveCommand(t, "snapshot-fast-path", 0))
	if err != nil || !receipt.Accepted || receipt.Projection == nil || receipt.Projection.Status != engine.ProjectionCurrent {
		t.Fatalf("start receipt=%#v err=%v", receipt, err)
	}
	store.transactionReads = 0

	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	resume := view.(engine.ResumeView)
	if resume.Capsule.LedgerRevision != 1 || resume.Capsule.Goal == "" {
		t.Fatalf("resume capsule = %#v", resume.Capsule)
	}
	if store.transactionReads != 2 {
		t.Fatalf("resume scanned canonical transactions %d times, want one pre-read and one stability validation", store.transactionReads)
	}
}

func TestResumeFallsBackToLedgerWhenLegacyHeadHasNoResumeDigest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatalf("new continuity module: %v", err)
	}
	store := &transactionCountingStore{Store: ledger.NewMemory()}
	objective := engine.Objective{
		ObjectiveID: "obj-legacy-resume", ProjectID: "project-legacy-resume",
		Title: "旧账本", Goal: "旧 transaction 必须通过完整 fold 恢复",
		Acceptance: []string{"恢复成功"}, Profile: "standard", Status: engine.ObjectiveActive,
		Revision: 1, Done: []string{}, Next: []string{"继续迁移"}, Validation: []string{}, Blockers: []string{}, MustRead: []string{},
	}
	data, err := json.Marshal(objective)
	if err != nil {
		t.Fatalf("marshal objective: %v", err)
	}
	actor, err := json.Marshal(engine.ActorRef{ActorID: "legacy-user", SessionID: "legacy-session", Runtime: "go-test", Role: engine.ActorUser})
	if err != nil {
		t.Fatalf("marshal actor: %v", err)
	}
	_, err = store.Commit(context.Background(), ledger.Draft{
		TransactionID: "tx-legacy-resume", ProjectID: objective.ProjectID,
		CommandID: "cmd-legacy-resume", CommandDigest: "legacy-command-digest", IdempotencyKey: "legacy-resume",
		CorrelationID: "corr-legacy-resume", IssuedAt: time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC), Actor: actor,
		Events: []ledger.Event{{EventID: "evt-legacy-resume", Kind: "ObjectiveStarted", EntityID: objective.ObjectiveID, Data: data}},
	}, 0)
	if err != nil {
		t.Fatalf("commit legacy transaction: %v", err)
	}
	kernel := engine.New(store, engine.WithContinuity(module))

	for attempt := 0; attempt < 2; attempt++ {
		before := store.transactionReads
		view, queryErr := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
		if queryErr != nil {
			t.Fatalf("resume attempt %d: %v", attempt, queryErr)
		}
		resume := view.(engine.ResumeView)
		if resume.Capsule.Goal != objective.Goal || resume.Capsule.LedgerRevision != 1 {
			t.Fatalf("resume capsule = %#v", resume.Capsule)
		}
		if store.transactionReads == before {
			t.Fatalf("resume attempt %d used an unverified snapshot for a legacy head", attempt)
		}
	}
}

func TestApplyHoldsLifecycleLockAcrossCanonicalCommit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatalf("new continuity module: %v", err)
	}
	store := &blockingCommitStore{
		Memory:  ledger.NewMemory(),
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}
	kernel := engine.New(store, engine.WithContinuity(module))
	command := startObjectiveCommand(t, "lifecycle-lock", 0)
	result := make(chan error, 1)
	go func() {
		receipt, applyErr := kernel.Apply(context.Background(), command)
		if applyErr == nil && (!receipt.Accepted || receipt.Projection == nil || receipt.Projection.Status != engine.ProjectionCurrent) {
			applyErr = fmt.Errorf("unexpected receipt: %#v", receipt)
		}
		result <- applyErr
	}()
	<-store.reached

	competitor, err := continuity.NewFile(root)
	if err != nil {
		t.Fatalf("new competing continuity module: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if unlock, lockErr := competitor.LockLifecycle(ctx); !errors.Is(lockErr, context.DeadlineExceeded) {
		if lockErr == nil {
			_ = unlock()
		}
		t.Fatalf("competing lifecycle lock error = %v, want deadline exceeded", lockErr)
	}
	close(store.release)
	if err := <-result; err != nil {
		t.Fatalf("apply after releasing commit: %v", err)
	}
}

func TestRetryingAnOlderCommandCannotRecreateAStaleHandoff(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatalf("new continuity module: %v", err)
	}
	store := ledger.NewMemory()
	kernel := engine.New(store, engine.WithContinuity(module))
	start := startObjectiveCommand(t, "stale-projection-retry", 0)
	started, err := kernel.Apply(context.Background(), start)
	if err != nil || !started.Accepted {
		t.Fatalf("start receipt=%#v err=%v", started, err)
	}
	nextValues := []string{"继续当前状态"}
	savePayload, err := json.Marshal(engine.SaveObjective{
		ObjectiveID: started.EntityID, ExpectedObjectiveRevision: 1, Done: []string{"已推进"}, Next: &nextValues,
	})
	if err != nil {
		t.Fatalf("marshal save: %v", err)
	}
	saved, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-save-after-start", IdempotencyKey: "save-after-start",
		CorrelationID: "corr-save-after-start", ProjectID: start.ProjectID, ExpectedRevision: 1,
		Actor: start.Actor, IssuedAt: start.IssuedAt.Add(time.Minute), Kind: engine.CommandSaveObjective, Payload: savePayload,
	})
	if err != nil || !saved.Accepted || saved.NewRevision != 2 {
		t.Fatalf("save receipt=%#v err=%v", saved, err)
	}
	if err := os.Remove(filepath.Join(root, ".agent", "HANDOFF.md")); err != nil {
		t.Fatalf("remove handoff: %v", err)
	}

	retry, err := kernel.Apply(context.Background(), start)
	if err != nil || !retry.Accepted || retry.TransactionID != started.TransactionID || retry.Projection == nil || retry.Projection.Status != engine.ProjectionCurrent {
		t.Fatalf("retry receipt=%#v err=%v", retry, err)
	}
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("resume after retry: %v", err)
	}
	resume := view.(engine.ResumeView)
	if resume.Capsule.LedgerRevision != 2 || resume.Capsule.Revision != 2 || !reflect.DeepEqual(resume.Capsule.Done, []string{"已推进"}) {
		t.Fatalf("resume capsule = %#v", resume.Capsule)
	}
}

func TestStartObjectiveRejectsSecondRootObjective(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	kernel := engine.New(store)
	first := startObjectiveCommand(t, "first", 0)
	if receipt, err := kernel.Apply(context.Background(), first); err != nil || !receipt.Accepted {
		t.Fatalf("start first objective receipt=%#v err=%v", receipt, err)
	}
	second := startObjectiveCommand(t, "second", 1)
	receipt, err := kernel.Apply(context.Background(), second)
	if err != nil {
		t.Fatalf("start second objective returned error: %v", err)
	}
	if receipt.Accepted {
		t.Fatalf("second root objective was accepted: %#v", receipt)
	}
	if receipt.Rejection == nil || receipt.Rejection.Code != "OBJECTIVE_EXISTS" {
		t.Fatalf("second objective rejection = %#v, want OBJECTIVE_EXISTS", receipt.Rejection)
	}
}

func TestStartObjectiveBindsDomainCheckToObservedRevision(t *testing.T) {
	t.Parallel()

	store := newFutureRevisionStore()
	kernel := engine.New(store)
	future := startObjectiveCommand(t, "future", 1)
	future.CommandID = "cmd-future"
	type result struct {
		receipt engine.Receipt
		err     error
	}
	futureResult := make(chan result, 1)
	go func() {
		receipt, err := kernel.Apply(context.Background(), future)
		futureResult <- result{receipt: receipt, err: err}
	}()

	var early *result
	select {
	case <-store.futureAtCommit:
	case got := <-futureResult:
		early = &got
	}

	current := startObjectiveCommand(t, "current", 0)
	currentReceipt, err := kernel.Apply(context.Background(), current)
	if err != nil || !currentReceipt.Accepted {
		t.Fatalf("current objective receipt=%#v err=%v", currentReceipt, err)
	}
	if early == nil {
		close(store.releaseFuture)
		got := <-futureResult
		early = &got
	}
	if early.err != nil {
		t.Fatalf("future command returned error: %v", early.err)
	}
	if early.receipt.Accepted {
		t.Fatalf("future-revision command used a stale domain snapshot: %#v", early.receipt)
	}
	transactions, err := store.Transactions(context.Background(), current.ProjectID)
	if err != nil {
		t.Fatalf("read transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("transactions = %d, want one Root Objective transaction", len(transactions))
	}
}

func TestConcurrentIdempotentStartReturnsCommittedReceipt(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "concurrent-retry", 0)
	const callers = 32
	receipts := make([]engine.Receipt, callers)
	errorsByCaller := make([]error, callers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(callers)
	for index := range callers {
		go func(index int) {
			defer wait.Done()
			<-start
			receipts[index], errorsByCaller[index] = kernel.Apply(context.Background(), command)
		}(index)
	}
	close(start)
	wait.Wait()

	transactionID := receipts[0].TransactionID
	entityID := receipts[0].EntityID
	if transactionID == "" || entityID == "" {
		t.Fatalf("first receipt is incomplete: %#v", receipts[0])
	}
	for index, err := range errorsByCaller {
		if err != nil {
			t.Fatalf("caller %d returned error: %v", index, err)
		}
		if !receipts[index].Accepted || receipts[index].TransactionID != transactionID || receipts[index].EntityID != entityID {
			t.Fatalf("caller %d receipt = %#v, want committed transaction %s entity %s", index, receipts[index], transactionID, entityID)
		}
	}
}

func TestEngineDoesNotCommitCanceledCommandWithMemoryLedger(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	kernel := engine.New(store)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := kernel.Apply(ctx, startObjectiveCommand(t, "canceled", 0)); !errors.Is(err, context.Canceled) {
		t.Fatalf("apply canceled command error = %v, want context.Canceled", err)
	}
	head, err := store.Head(context.Background(), "project-one-objective")
	if err != nil {
		t.Fatalf("read memory head: %v", err)
	}
	if head.Revision != 0 {
		t.Fatalf("memory ledger revision = %d after canceled command, want 0", head.Revision)
	}
}

func TestWorkerCannotStartRootObjective(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "worker", 0)
	command.Actor.Role = engine.ActorWorker
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("worker start returned error: %v", err)
	}
	if receipt.Accepted {
		t.Fatalf("worker start was accepted: %#v", receipt)
	}
	if receipt.Rejection == nil || receipt.Rejection.Code != "FORBIDDEN" {
		t.Fatalf("worker rejection = %#v, want FORBIDDEN", receipt.Rejection)
	}
}

func TestInvalidActorRoleReturnsMachineReadableRejection(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "invalid-role", 0)
	command.Actor.Role = engine.ActorRole("admin")
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("invalid role returned infrastructure error: %v", err)
	}
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "INVALID_ACTOR" {
		t.Fatalf("invalid role receipt = %#v, want INVALID_ACTOR", receipt)
	}
}

func TestStartObjectiveRejectsOversizedCanonicalText(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "oversized", 0)
	payload, err := json.Marshal(engine.StartObjective{
		Title:      strings.Repeat("x", 2001),
		Goal:       "限制 Canonical Ledger 输入大小",
		Acceptance: []string{"返回 INVALID_COMMAND"},
		Profile:    "standard",
	})
	if err != nil {
		t.Fatalf("marshal oversized objective: %v", err)
	}
	command.Payload = payload
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("oversized objective returned infrastructure error: %v", err)
	}
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "INVALID_COMMAND" {
		t.Fatalf("oversized objective receipt = %#v, want INVALID_COMMAND", receipt)
	}
}

func TestStartObjectiveRejectsHighConfidenceSecret(t *testing.T) {
	t.Parallel()

	secrets := []string{
		"sk-proj-" + strings.Repeat("A", 32),
		"github_pat_" + strings.Repeat("B", 32),
		"glpat-" + strings.Repeat("C", 24),
		"xoxb-" + strings.Repeat("1", 12) + "-" + strings.Repeat("D", 24),
		"sk_live_" + strings.Repeat("E", 24),
	}
	for index, secret := range secrets {
		kernel := engine.New(ledger.NewMemory())
		command := startObjectiveCommand(t, fmt.Sprintf("secret-%d", index), 0)
		payload, err := json.Marshal(engine.StartObjective{
			Title:      "不要把密钥写进 Ledger",
			Goal:       "token=" + secret,
			Acceptance: []string{"返回 SENSITIVE_CONTENT"},
			Profile:    "standard",
		})
		if err != nil {
			t.Fatalf("marshal secret objective: %v", err)
		}
		command.Payload = payload
		receipt, err := kernel.Apply(context.Background(), command)
		if err != nil {
			t.Fatalf("secret objective returned infrastructure error: %v", err)
		}
		if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "SENSITIVE_CONTENT" {
			t.Fatalf("secret objective receipt = %#v, want SENSITIVE_CONTENT", receipt)
		}
	}
}

func TestStartObjectiveRejectsHighConfidenceSecretInProvenance(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "secret-provenance", 0)
	command.Actor.SessionID = "github_pat_" + strings.Repeat("A", 32)
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("secret provenance returned infrastructure error: %v", err)
	}
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "SENSITIVE_CONTENT" {
		t.Fatalf("secret provenance receipt = %#v, want SENSITIVE_CONTENT", receipt)
	}
}

func TestStartObjectiveRejectsEscapedSecretInPayload(t *testing.T) {
	t.Parallel()

	kernel := engine.New(ledger.NewMemory())
	command := startObjectiveCommand(t, "escaped-secret", 0)
	command.Payload = json.RawMessage(`{"title":"不要写密钥","goal":"token=\u0073k-proj-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","acceptance":["返回 SENSITIVE_CONTENT"],"profile":"standard"}`)
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("escaped secret returned infrastructure error: %v", err)
	}
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "SENSITIVE_CONTENT" {
		t.Fatalf("escaped secret receipt = %#v, want SENSITIVE_CONTENT", receipt)
	}
}

func startObjectiveCommand(t *testing.T, suffix string, expectedRevision uint64) engine.CommandEnvelope {
	t.Helper()
	payload, err := json.Marshal(engine.StartObjective{
		Title:      "目标 " + suffix,
		Goal:       "一个项目只有一个 Root Objective",
		Acceptance: []string{"拒绝第二个顶层目标"},
		Profile:    "standard",
	})
	if err != nil {
		t.Fatalf("marshal start objective: %v", err)
	}
	return engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-" + suffix,
		IdempotencyKey:   "start-" + suffix,
		CorrelationID:    "corr-" + suffix,
		IssuedAt:         time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
		ProjectID:        "project-one-objective",
		ExpectedRevision: expectedRevision,
		Actor: engine.ActorRef{
			ActorID:   "user-summer",
			SessionID: "session-" + suffix,
			Runtime:   "codex",
			Role:      engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: payload,
	}
}

type transactionCountingStore struct {
	ledger.Store
	transactionReads int
}

type blockingCommitStore struct {
	*ledger.Memory
	reached chan struct{}
	release chan struct{}
}

func (s *blockingCommitStore) Commit(ctx context.Context, draft ledger.Draft, expectedRevision uint64) (ledger.Transaction, error) {
	close(s.reached)
	select {
	case <-ctx.Done():
		return ledger.Transaction{}, ctx.Err()
	case <-s.release:
		return s.Memory.Commit(ctx, draft, expectedRevision)
	}
}

func (s *transactionCountingStore) Transactions(ctx context.Context, projectID string) ([]ledger.Transaction, error) {
	s.transactionReads++
	return s.Store.Transactions(ctx, projectID)
}

type futureRevisionStore struct {
	*ledger.Memory
	futureAtCommit chan struct{}
	releaseFuture  chan struct{}
	once           sync.Once
}

func newFutureRevisionStore() *futureRevisionStore {
	return &futureRevisionStore{
		Memory:         ledger.NewMemory(),
		futureAtCommit: make(chan struct{}),
		releaseFuture:  make(chan struct{}),
	}
}

func (s *futureRevisionStore) Commit(ctx context.Context, draft ledger.Draft, expectedRevision uint64) (ledger.Transaction, error) {
	if draft.CommandID == "cmd-future" {
		s.once.Do(func() { close(s.futureAtCommit) })
		select {
		case <-ctx.Done():
			return ledger.Transaction{}, ctx.Err()
		case <-s.releaseFuture:
		}
	}
	return s.Memory.Commit(ctx, draft, expectedRevision)
}
