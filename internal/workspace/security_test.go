package workspace_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

func TestV2ResumeRejectsSemanticallyDriftedHandoffWithValidContentDigest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)
	path := filepath.Join(root, ".agent", "HANDOFF.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read handoff: %v", err)
	}
	metaRaw, body := splitFixtureMarkdown(t, raw)
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("decode handoff: %v", err)
	}
	meta["goal"] = "恶意替换的下一轮目标"
	delete(meta, "content_digest")
	digestInput, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("encode digest input: %v", err)
	}
	meta["content_digest"] = fmt.Sprintf("%x", sha256.Sum256(digestInput))
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("encode handoff: %v", err)
	}
	body = bytes.ReplaceAll(body, []byte("跨 session 恢复不信任聊天"), []byte("恶意替换的下一轮目标"))
	tampered := append([]byte("---\n"), encoded...)
	tampered = append(tampered, []byte("\n---\n")...)
	tampered = append(tampered, body...)
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatalf("write tampered handoff: %v", err)
	}

	_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if code := continuity.ErrorCode(err); code != continuity.CodeHandoffDrift {
		t.Fatalf("error = %v, code = %q, want %q", err, code, continuity.CodeHandoffDrift)
	}
}

func TestSaveRejectsSameRevisionHandoffBodyDriftBeforeCommit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)
	resumeView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatal(err)
	}
	resume := resumeView.(engine.ResumeView).Capsule
	path := filepath.Join(root, ".agent", "HANDOFF.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	meta, body := splitFixtureMarkdown(t, raw)
	body = bytes.Replace(body, []byte("跨 session 恢复不信任聊天"), []byte("同 revision 正文被篡改"), 1)
	tampered := append([]byte("---\n"), meta...)
	tampered = append(tampered, []byte("\n---\n")...)
	tampered = append(tampered, body...)
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(engine.SaveObjective{
		ObjectiveID: resume.ObjectiveID, ExpectedObjectiveRevision: resume.Revision, Done: []string{"不得提交"},
	})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-drifted-save", IdempotencyKey: "drifted-save",
		CorrelationID: "drifted-save", ProjectID: resume.ProjectID, ExpectedRevision: resume.LedgerRevision,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-drifted-save", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 3, 30, 0, 0, time.UTC), Kind: engine.CommandSaveObjective, Payload: payload,
	})
	if err != nil || receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeHandoffDrift) {
		t.Fatalf("save receipt=%#v err=%v", receipt, err)
	}
	manifests := readFixtureLedgerManifests(t, root)
	if len(manifests) != 1 || manifests[1].Revision != 1 {
		t.Fatalf("canonical ledger advanced after drift: %#v", manifests)
	}
}

func TestV2ResumeRejectsMatchingTamperedSnapshotAndHandoff(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)

	snapshotPath := filepath.Join(root, ".agent", "cache", "resume.snapshot.json")
	snapshotRaw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snapshot fixtureResumeSnapshot
	if err := json.Unmarshal(snapshotRaw, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	snapshot.State.Goal = "伪造但未进入 Canonical Ledger 的目标"
	digestState := snapshot.State
	digestState.BuiltAt = time.Time{}
	stateRaw, err := json.Marshal(digestState)
	if err != nil {
		t.Fatalf("encode snapshot state: %v", err)
	}
	snapshot.StateDigest = fmt.Sprintf("%x", sha256.Sum256(stateRaw))
	snapshot.ContentDigest = ""
	snapshotDigestInput, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot digest input: %v", err)
	}
	snapshot.ContentDigest = fmt.Sprintf("%x", sha256.Sum256(snapshotDigestInput))
	encodedSnapshot, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered snapshot: %v", err)
	}
	if err := os.WriteFile(snapshotPath, append(encodedSnapshot, '\n'), 0o600); err != nil {
		t.Fatalf("write tampered snapshot: %v", err)
	}

	handoffPath := filepath.Join(root, ".agent", "HANDOFF.md")
	handoffRaw, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("read handoff: %v", err)
	}
	metaRaw, body := splitFixtureMarkdown(t, handoffRaw)
	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		t.Fatalf("decode handoff: %v", err)
	}
	meta["goal"] = snapshot.State.Goal
	delete(meta, "content_digest")
	handoffDigestInput, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("encode handoff digest input: %v", err)
	}
	meta["content_digest"] = fmt.Sprintf("%x", sha256.Sum256(handoffDigestInput))
	encodedMeta, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered handoff: %v", err)
	}
	body = bytes.ReplaceAll(body, []byte("跨 session 恢复不信任聊天"), []byte(snapshot.State.Goal))
	tamperedHandoff := append([]byte("---\n"), encodedMeta...)
	tamperedHandoff = append(tamperedHandoff, []byte("\n---\n")...)
	tamperedHandoff = append(tamperedHandoff, body...)
	if err := os.WriteFile(handoffPath, tamperedHandoff, 0o644); err != nil {
		t.Fatalf("write tampered handoff: %v", err)
	}

	_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if code := continuity.ErrorCode(err); code != continuity.CodeHandoffDrift {
		t.Fatalf("error = %v, code = %q, want %q", err, code, continuity.CodeHandoffDrift)
	}
}

func TestResumeRejectsOversizedAndDuplicateKeyHandoffs(t *testing.T) {
	t.Parallel()

	t.Run("oversized", func(t *testing.T) {
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), strings.Repeat(" ", continuity.HandoffLimit+1))
		kernel, err := workspace.Open(root)
		if err != nil {
			t.Fatalf("open workspace: %v", err)
		}
		_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
		if code := continuity.ErrorCode(err); code != continuity.CodeHandoffTooLarge {
			t.Fatalf("error = %v, code = %q", err, code)
		}
	})

	t.Run("duplicate-key", func(t *testing.T) {
		root := t.TempDir()
		kernel := startSecurityFixture(t, root)
		path := filepath.Join(root, ".agent", "HANDOFF.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read handoff: %v", err)
		}
		raw = bytes.Replace(raw, []byte(`"schema": "summer.handoff/v2"`), []byte("\"schema\": \"summer.handoff/v2\",\n  \"schema\": \"summer.handoff/v2\""), 1)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatalf("write handoff: %v", err)
		}
		_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
		if code := continuity.ErrorCode(err); code != continuity.CodeHandoffInvalid {
			t.Fatalf("error = %v, code = %q", err, code)
		}
	})

	t.Run("handoff-symlink", func(t *testing.T) {
		root := t.TempDir()
		external := filepath.Join(t.TempDir(), "HANDOFF.md")
		if err := os.WriteFile(external, []byte("not trusted\n"), 0o644); err != nil {
			t.Fatalf("write external handoff: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".agent"), 0o755); err != nil {
			t.Fatalf("create .agent: %v", err)
		}
		if err := os.Symlink(external, filepath.Join(root, ".agent", "HANDOFF.md")); err != nil {
			t.Fatalf("create handoff symlink: %v", err)
		}
		kernel, err := workspace.Open(root)
		if err != nil {
			t.Fatalf("open workspace: %v", err)
		}
		_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
		if code := continuity.ErrorCode(err); code != continuity.CodeUnsafeReference {
			t.Fatalf("error = %v, code = %q", err, code)
		}
	})
}

func TestV2ResumeRebuildsOnlyAMissingHandoffFromCanonicalLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)
	path := filepath.Join(root, ".agent", "HANDOFF.md")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove handoff: %v", err)
	}
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("resume and rebuild missing handoff: %v", err)
	}
	resume := view.(engine.ResumeView)
	if resume.Capsule.Schema != continuity.CapsuleSchemaV2 || resume.Capsule.LedgerHead == "" {
		t.Fatalf("rebuilt capsule = %#v", resume.Capsule)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 || info.Size() > continuity.HandoffLimit {
		t.Fatalf("rebuilt handoff info=%#v err=%v", info, err)
	}
}

func TestResumeAdoptsAuthorizedPendingTransactionBeforeUsingSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)
	handoffV1, err := os.ReadFile(filepath.Join(root, ".agent", "HANDOFF.md"))
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(root, ".agent", "cache", "resume.snapshot.json")
	snapshotV1, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	resumeView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatal(err)
	}
	resume := resumeView.(engine.ResumeView).Capsule
	next := []string{"恢复 revision 2"}
	payload, _ := json.Marshal(engine.SaveObjective{
		ObjectiveID: resume.ObjectiveID, ExpectedObjectiveRevision: resume.Revision,
		Done: []string{"transaction 已落盘"}, Next: &next,
	})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-pending-recovery", IdempotencyKey: "pending-recovery",
		CorrelationID: "pending-recovery", ProjectID: resume.ProjectID, ExpectedRevision: resume.LedgerRevision,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-pending", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC), Kind: engine.CommandSaveObjective, Payload: payload,
	})
	if err != nil || !receipt.Accepted || receipt.NewRevision != 2 {
		t.Fatalf("save receipt=%#v err=%v", receipt, err)
	}
	manifests := readFixtureLedgerManifests(t, root)
	first, second := manifests[1], manifests[2]
	writeFixtureJSON(t, filepath.Join(root, ".agent", "ledger", "HEAD"), map[string]any{
		"schema": "summer.ledger-head/v2", "project_id": first.ProjectID,
		"transaction_id": first.TransactionID, "revision": first.Revision,
		"digest": first.Digest, "resume_digest": first.ResumeDigest,
	})
	writeFixtureJSON(t, filepath.Join(root, ".agent", "runtime", "ledger.pending.json"), map[string]any{
		"schema": "summer.pending-commit/v2", "project_id": second.ProjectID,
		"transaction_id": second.TransactionID, "revision": second.Revision,
		"previous_digest": second.PreviousDigest, "digest": second.Digest,
	})
	if err := os.WriteFile(filepath.Join(root, ".agent", "HANDOFF.md"), handoffV1, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, snapshotV1, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	view, err := reopened.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("resume pending transaction: %v", err)
	}
	capsule := view.(engine.ResumeView).Capsule
	if capsule.LedgerRevision != 2 || !reflect.DeepEqual(capsule.Done, []string{"transaction 已落盘"}) {
		t.Fatalf("capsule=%#v", capsule)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "runtime", "ledger.pending.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending marker remains: %v", err)
	}
}

func TestResumeRejectsSnapshotWhenLedgerPredecessorIsMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel := startSecurityFixture(t, root)
	resumeView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatal(err)
	}
	resume := resumeView.(engine.ResumeView).Capsule
	payload, _ := json.Marshal(engine.SaveObjective{ObjectiveID: resume.ObjectiveID, ExpectedObjectiveRevision: resume.Revision, Done: []string{"revision 2"}})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-broken-chain", IdempotencyKey: "broken-chain",
		CorrelationID: "broken-chain", ProjectID: resume.ProjectID, ExpectedRevision: resume.LedgerRevision,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-broken-chain", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 4, 30, 0, 0, time.UTC), Kind: engine.CommandSaveObjective, Payload: payload,
	})
	if err != nil || !receipt.Accepted || receipt.NewRevision != 2 {
		t.Fatalf("save receipt=%#v err=%v", receipt, err)
	}
	first := readFixtureLedgerManifests(t, root)[1]
	if err := os.RemoveAll(filepath.Join(root, ".agent", "ledger", "transactions", first.TransactionID)); err != nil {
		t.Fatal(err)
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reopened.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err == nil || !strings.Contains(err.Error(), "valid predecessor") {
		t.Fatalf("resume error=%v, want broken predecessor rejection", err)
	}
}

func TestStartObjectiveRejectsInvalidLegacyHandoffBeforeOpeningV2Ledger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), "---\n{\n  \"schema\": \"summer-harness/v1\"\n}\n---\n# Legacy handoff\n")
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	receipt := applySecurityObjective(t, kernel, "legacy-conflict", "不能建立第二生命周期", []string{"要求显式迁移"})
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeHandoffInvalid) {
		t.Fatalf("receipt = %#v, want HANDOFF_INVALID", receipt)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "ledger", "HEAD")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy conflict created canonical HEAD: %v", err)
	}
}

func TestCommittedCommandRetryRemainsAcceptedWhenProjectionLaterConflicts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	command := securityObjectiveCommand(t, "idempotent-projection", "保留原始提交结果", []string{"幂等重试仍 accepted"})
	first, err := kernel.Apply(context.Background(), command)
	if err != nil || !first.Accepted {
		t.Fatalf("first apply receipt=%#v err=%v", first, err)
	}
	mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), "---\n{\n  \"schema\": \"summer-harness/v1\"\n}\n---\n# Conflicting legacy handoff\n")
	retry, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if !retry.Accepted || retry.TransactionID != first.TransactionID || retry.Projection == nil || retry.Projection.Status != engine.ProjectionRepairRequired {
		t.Fatalf("retry receipt=%#v, want original accepted receipt with repair_required", retry)
	}
}

func TestOrphanV2HandoffCannotCreateACanonicalLedger(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	_ = startSecurityFixture(t, sourceRoot)
	raw, err := os.ReadFile(filepath.Join(sourceRoot, ".agent", "HANDOFF.md"))
	if err != nil {
		t.Fatalf("read source handoff: %v", err)
	}
	targetRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(targetRoot, ".agent"), 0o755); err != nil {
		t.Fatalf("create target .agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, ".agent", "HANDOFF.md"), raw, 0o644); err != nil {
		t.Fatalf("copy orphan handoff: %v", err)
	}
	kernel, err := workspace.Open(targetRoot)
	if err != nil {
		t.Fatalf("open target workspace: %v", err)
	}
	command := securityObjectiveCommand(t, "orphan-handoff", "不能信任孤立投影", []string{"提交前拒绝"})
	command.ProjectID = "project-security"
	receipt, err := kernel.Apply(context.Background(), command)
	if err != nil {
		t.Fatalf("apply objective: %v", err)
	}
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeLifecycleConflict) {
		t.Fatalf("receipt=%#v, want LIFECYCLE_CONFLICT", receipt)
	}
	assertNoCanonicalHead(t, targetRoot)
}

func TestResumeRejectsSnapshotCacheDirectorySymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_ = startSecurityFixture(t, root)
	cache := filepath.Join(root, ".agent", "cache")
	snapshotRaw, err := os.ReadFile(filepath.Join(cache, "resume.snapshot.json"))
	if err != nil {
		t.Fatalf("read generated snapshot: %v", err)
	}
	if err := os.RemoveAll(cache); err != nil {
		t.Fatalf("remove cache: %v", err)
	}
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "resume.snapshot.json"), string(snapshotRaw))
	if err := os.Symlink(outside, cache); err != nil {
		t.Fatalf("symlink cache: %v", err)
	}

	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if continuity.ErrorCode(err) != continuity.CodeUnsafeReference {
		t.Fatalf("resume error = %v, want UNSAFE_REFERENCE", err)
	}
}

func TestStartObjectiveRejectsUnprojectableCanonicalStateBeforeCommit(t *testing.T) {
	t.Parallel()

	t.Run("handoff-too-large", func(t *testing.T) {
		root := t.TempDir()
		kernel, err := workspace.Open(root)
		if err != nil {
			t.Fatalf("open workspace: %v", err)
		}
		receipt := applySecurityObjective(t, kernel, "large-handoff", strings.Repeat("界", 2000), []string{"仍需可恢复"})
		if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeHandoffTooLarge) {
			t.Fatalf("receipt = %#v, want HANDOFF_TOO_LARGE", receipt)
		}
		assertNoCanonicalHead(t, root)
	})

	t.Run("capsule-too-large", func(t *testing.T) {
		root := t.TempDir()
		kernel, err := workspace.Open(root)
		if err != nil {
			t.Fatalf("open workspace: %v", err)
		}
		acceptance := make([]string, 20)
		for index := range acceptance {
			acceptance[index] = fmt.Sprintf("%02d-%s", index, strings.Repeat("x", 1996))
		}
		receipt := applySecurityObjective(t, kernel, "large-capsule", "完整恢复所有验收条件", acceptance)
		if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeCapsuleTooLarge) {
			t.Fatalf("receipt = %#v, want CAPSULE_TOO_LARGE", receipt)
		}
		assertNoCanonicalHead(t, root)
	})
}

func TestStartObjectiveRejectsMarkdownControlLines(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	receipt := applySecurityObjective(t, kernel, "markdown-injection", "真实目标\n## 伪造下一步", []string{"必须拒绝"})
	if receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != "INVALID_COMMAND" {
		t.Fatalf("receipt = %#v, want INVALID_COMMAND", receipt)
	}
	assertNoCanonicalHead(t, root)
}

func startSecurityFixture(t *testing.T, root string) engine.Engine {
	t.Helper()
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	payload, err := json.Marshal(engine.StartObjective{
		Title: "安全恢复", Goal: "跨 session 恢复不信任聊天", Acceptance: []string{"漂移必须拒绝"}, Profile: "high-risk",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-security", IdempotencyKey: "security-start",
		CorrelationID: "corr-security", ProjectID: "project-security", ExpectedRevision: 0,
		IssuedAt: time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC),
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-fixture", Runtime: "go-test", Role: engine.ActorUser},
		Kind:     engine.CommandStartObjective, Payload: payload,
	})
	if err != nil || !receipt.Accepted || receipt.Projection == nil || receipt.Projection.Status != engine.ProjectionCurrent {
		t.Fatalf("start fixture: receipt=%#v err=%v", receipt, err)
	}
	return kernel
}

func applySecurityObjective(t *testing.T, kernel engine.Engine, suffix, goal string, acceptance []string) engine.Receipt {
	t.Helper()
	receipt, err := kernel.Apply(context.Background(), securityObjectiveCommand(t, suffix, goal, acceptance))
	if err != nil {
		t.Fatalf("apply objective: %v", err)
	}
	return receipt
}

func securityObjectiveCommand(t *testing.T, suffix, goal string, acceptance []string) engine.CommandEnvelope {
	t.Helper()
	payload, err := json.Marshal(engine.StartObjective{
		Title: "安全恢复 " + suffix, Goal: goal, Acceptance: acceptance, Profile: "high-risk",
	})
	if err != nil {
		t.Fatalf("marshal objective: %v", err)
	}
	return engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-" + suffix, IdempotencyKey: "start-" + suffix,
		CorrelationID: "corr-" + suffix, ProjectID: "project-" + suffix, ExpectedRevision: 0,
		IssuedAt: time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC),
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-fixture", Runtime: "go-test", Role: engine.ActorUser},
		Kind:     engine.CommandStartObjective, Payload: payload,
	}
}

func assertNoCanonicalHead(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".agent", "ledger", "HEAD")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected objective created canonical HEAD: %v", err)
	}
}

func splitFixtureMarkdown(t *testing.T, raw []byte) ([]byte, []byte) {
	t.Helper()
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		t.Fatal("handoff has no opening fence")
	}
	end := bytes.Index(raw[4:], []byte("\n---\n"))
	if end < 0 {
		t.Fatal("handoff has no closing fence")
	}
	return raw[4 : 4+end], raw[4+end+5:]
}

type fixtureResumeSnapshot struct {
	Schema           string                     `json:"schema"`
	ProjectID        string                     `json:"project_id"`
	LedgerRevision   uint64                     `json:"ledger_revision"`
	LedgerHead       string                     `json:"ledger_head"`
	ProjectorVersion int                        `json:"projector_version"`
	BuiltAt          time.Time                  `json:"built_at"`
	StateDigest      string                     `json:"state_digest"`
	State            fixtureResumeSnapshotState `json:"state"`
	ContentDigest    string                     `json:"content_digest"`
}

type fixtureResumeSnapshotState struct {
	ProjectID         string    `json:"project_id"`
	ObjectiveID       string    `json:"objective_id"`
	ObjectiveStatus   string    `json:"objective_status"`
	ObjectiveRevision uint64    `json:"objective_revision"`
	Goal              string    `json:"goal"`
	Profile           string    `json:"profile"`
	Acceptance        []string  `json:"acceptance"`
	Done              []string  `json:"done"`
	Next              []string  `json:"next"`
	Validation        []string  `json:"validation"`
	Blockers          []string  `json:"blockers"`
	MustRead          []string  `json:"must_read"`
	ResumeCommand     string    `json:"resume_command"`
	BuiltAt           time.Time `json:"built_at"`
}

type fixtureLedgerManifest struct {
	TransactionID  string `json:"transaction_id"`
	ProjectID      string `json:"project_id"`
	ResumeDigest   string `json:"resume_digest"`
	Revision       uint64 `json:"revision"`
	PreviousDigest string `json:"previous_digest"`
	Digest         string `json:"digest"`
}

func readFixtureLedgerManifests(t *testing.T, root string) map[uint64]fixtureLedgerManifest {
	t.Helper()
	directory := filepath.Join(root, ".agent", "ledger", "transactions")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[uint64]fixtureLedgerManifest, len(entries))
	for _, entry := range entries {
		var manifest fixtureLedgerManifest
		raw, err := os.ReadFile(filepath.Join(directory, entry.Name(), "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		result[manifest.Revision] = manifest
	}
	return result
}

func writeFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
