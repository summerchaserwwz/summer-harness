package continuity_test

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

	"github.com/summerchaserwwz/summer-harness/internal/cli"
	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/ledger"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

func TestLegacyMigrationDryRunImportsFullHistoryAndSwitchesAtomically(t *testing.T) {
	root := t.TempDir()
	fixture := writeLegacyMigrationFixture(t, root, 13)
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}

	beforeHandoff := mustRead(t, filepath.Join(root, ".agent", "HANDOFF.md"))
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration})
	if err != nil {
		t.Fatalf("preview migration: %v", err)
	}
	preview := view.(engine.LegacyMigrationView)
	if preview.Committed {
		t.Fatal("dry-run reported a committed migration")
	}
	wantCounts := continuity.LegacyMigrationCounts{Objectives: 2, Decisions: 4, Facts: 13, FactInvalidations: 1}
	if !reflect.DeepEqual(preview.Migration.Counts, wantCounts) {
		t.Fatalf("preview counts = %#v, want %#v", preview.Migration.Counts, wantCounts)
	}
	if got := mustRead(t, filepath.Join(root, ".agent", "HANDOFF.md")); !bytes.Equal(got, beforeHandoff) {
		t.Fatal("dry-run changed HANDOFF.md")
	}
	for _, path := range []string{
		filepath.Join(root, ".agent", "ledger", "HEAD"),
		filepath.Join(root, ".agent", "ledger", "transactions"),
		filepath.Join(root, ".agent", "archive"),
		filepath.Join(root, ".agent", "runtime"),
	} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("dry-run created %s: %v", path, statErr)
		}
	}

	payload, err := json.Marshal(engine.ImportLegacyNative{
		MigrationID: preview.Migration.MigrationID, SourceDigest: preview.Migration.SourceDigest,
		BackupManifestDigest: preview.Migration.BackupManifestDigest,
	})
	if err != nil {
		t.Fatalf("marshal migration command: %v", err)
	}
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-migrate-fixture", IdempotencyKey: "migrate-fixture",
		CorrelationID: "migrate-fixture", ProjectID: preview.Migration.ProjectID, ExpectedRevision: 0,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-fixture", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), Kind: engine.CommandImportLegacyNative, Payload: payload,
	})
	if err != nil || !receipt.Accepted || receipt.NewRevision != 1 || receipt.Projection == nil || receipt.Projection.Status != engine.ProjectionCurrent {
		t.Fatalf("migration receipt=%#v rejection=%#v err=%v", receipt, receipt.Rejection, err)
	}

	resumeView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("resume migrated project: %v", err)
	}
	resume := resumeView.(engine.ResumeView).Capsule
	if resume.Schema != continuity.CapsuleSchemaV2 || resume.ObjectiveID != preview.Migration.ActiveObjectiveID || resume.Goal != fixture.activeGoal || resume.Revision != 3 {
		t.Fatalf("migrated resume = %#v", resume)
	}

	committedView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration, ProjectID: preview.Migration.ProjectID})
	if err != nil {
		t.Fatalf("query committed migration: %v", err)
	}
	committed := committedView.(engine.LegacyMigrationView)
	if !committed.Committed || committed.LedgerRevision != 1 || committed.Migration.SemanticDigest != preview.Migration.SemanticDigest || !reflect.DeepEqual(committed.Migration.Counts, wantCounts) {
		t.Fatalf("committed migration = %#v", committed)
	}
	var completedLegacy map[string]any
	for _, record := range committed.Migration.Records {
		if record.EventKind == "ObjectiveImported" && record.LegacyTaskID == fixture.completedTaskID {
			if err := json.Unmarshal(record.Data, &completedLegacy); err != nil {
				t.Fatalf("decode completed legacy task: %v", err)
			}
		}
	}
	review, _ := completedLegacy["review"].(map[string]any)
	closeout, _ := completedLegacy["closeout"].(map[string]any)
	if review["reviewed_by"] != "reviewer-fixture" || closeout["completed_by"] != "user-fixture" || closeout["residual_acknowledged"] != true {
		t.Fatalf("review/closeout were not preserved: review=%#v closeout=%#v", review, closeout)
	}

	backupHandoff := filepath.Join(root, ".agent", "archive", "migrations", preview.Migration.MigrationID, "v1", "HANDOFF.md")
	if got := mustRead(t, backupHandoff); !bytes.Equal(got, beforeHandoff) {
		t.Fatal("backup did not preserve the original handoff bytes")
	}
	if err := os.WriteFile(filepath.Join(root, ".agent", "HANDOFF.md"), beforeHandoff, 0o644); err != nil {
		t.Fatalf("restore legacy handoff to simulate interrupted switch: %v", err)
	}
	pendingView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration})
	if err != nil || !pendingView.(engine.LegacyMigrationView).SwitchPending {
		t.Fatalf("migration switch pending view=%#v err=%v", pendingView, err)
	}
	next := []string{"不得在切换完成前推进"}
	savePayload, err := json.Marshal(engine.SaveObjective{
		ObjectiveID: preview.Migration.ActiveObjectiveID, ExpectedObjectiveRevision: 3, Next: &next,
	})
	if err != nil {
		t.Fatal(err)
	}
	saveReceipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-save-during-switch", IdempotencyKey: "save-during-switch",
		CorrelationID: "save-during-switch", ProjectID: preview.Migration.ProjectID, ExpectedRevision: 1,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-switch", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 12, 0, 30, 0, time.UTC), Kind: engine.CommandSaveObjective, Payload: savePayload,
	})
	if err != nil || saveReceipt.Accepted || saveReceipt.Rejection == nil || saveReceipt.Rejection.Code != string(continuity.CodeMigrationRequired) {
		t.Fatalf("save during migration switch receipt=%#v err=%v", saveReceipt, err)
	}
	var retryOut, retryErr bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"migrate", "--repo", root, "--json"}, root, &retryOut, &retryErr); exit != 0 || retryErr.Len() != 0 {
		t.Fatalf("migration retry exit=%d stdout=%s stderr=%s", exit, retryOut.String(), retryErr.String())
	}
	repairedView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil || repairedView.(engine.ResumeView).Capsule.Schema != continuity.CapsuleSchemaV2 {
		t.Fatalf("migration retry did not repair v2 handoff: view=%#v err=%v", repairedView, err)
	}

	rollbackView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
	if err != nil {
		t.Fatalf("preview rollback: %v", err)
	}
	rollback := rollbackView.(engine.LegacyRollbackView).Rollback
	rollbackPayload, err := json.Marshal(engine.RollbackLegacyMigration{
		MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
	})
	if err != nil {
		t.Fatal(err)
	}
	rollbackCommand := engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-rollback-fixture", IdempotencyKey: "rollback-fixture",
		CorrelationID: "rollback-fixture", ProjectID: rollback.ProjectID, ExpectedRevision: 1,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-rollback", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 12, 1, 0, 0, time.UTC), Kind: engine.CommandRollbackLegacyMigration, Payload: rollbackPayload,
	}
	rolledBack, err := kernel.Apply(context.Background(), rollbackCommand)
	if err != nil || !rolledBack.Accepted || rolledBack.EntityStatus != "rolled_back" {
		t.Fatalf("rollback receipt=%#v err=%v", rolledBack, err)
	}
	if got := mustRead(t, filepath.Join(root, ".agent", "HANDOFF.md")); !bytes.Equal(got, beforeHandoff) {
		t.Fatal("rollback did not restore the original handoff bytes")
	}
	if _, err := os.Lstat(filepath.Join(root, ".agent", "ledger", "HEAD")); !os.IsNotExist(err) {
		t.Fatalf("rollback left live HEAD: %v", err)
	}
	quarantined := filepath.Join(root, ".agent", "archive", "migrations", rollback.MigrationID, "rollback", "v2", "transactions", rollback.TransactionID)
	if info, err := os.Stat(quarantined); err != nil || !info.IsDir() {
		t.Fatalf("migration transaction was not quarantined: %v", err)
	}
	legacyResume, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil || legacyResume.(engine.ResumeView).Capsule.Schema != "summer-harness/v1" {
		t.Fatalf("legacy resume after rollback=%#v err=%v", legacyResume, err)
	}
	rollbackCommand.CommandID = "cmd-rollback-retry"
	rollbackCommand.IdempotencyKey = "rollback-retry"
	rollbackCommand.CorrelationID = "rollback-retry"
	rolledBack, err = kernel.Apply(context.Background(), rollbackCommand)
	if err != nil || !rolledBack.Accepted {
		t.Fatalf("idempotent rollback retry receipt=%#v err=%v", rolledBack, err)
	}
}

func TestLegacyRollbackRejectsSymlinkedSnapshotCacheBeforeQuarantine(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 2)
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := previewLegacyMigration(t, kernel)
	applyLegacyMigration(t, kernel, preview, "rollback-cache-symlink")
	rollbackView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
	if err != nil {
		t.Fatal(err)
	}
	rollback := rollbackView.(engine.LegacyRollbackView).Rollback
	cache := filepath.Join(root, ".agent", "cache")
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	externalSnapshot := filepath.Join(external, "resume.snapshot.json")
	if err := os.WriteFile(externalSnapshot, []byte("external evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, cache); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(engine.RollbackLegacyMigration{
		MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
	})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-rollback-cache-symlink", IdempotencyKey: "rollback-cache-symlink",
		CorrelationID: "rollback-cache-symlink", ProjectID: rollback.ProjectID, ExpectedRevision: 1,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-cache-symlink", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 15, 30, 0, 0, time.UTC), Kind: engine.CommandRollbackLegacyMigration, Payload: payload,
	})
	if err != nil || receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeUnsafeReference) {
		t.Fatalf("rollback receipt=%#v err=%v", receipt, err)
	}
	if got := mustRead(t, externalSnapshot); string(got) != "external evidence\n" {
		t.Fatalf("external snapshot changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "ledger", "HEAD")); err != nil {
		t.Fatalf("genesis was quarantined before path rejection: %v", err)
	}
}

func TestLegacyMigrationRejectsSymlinkedLedgerDirectory(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeLegacyMigrationFixture(t, root, 1)
	if err := os.RemoveAll(filepath.Join(root, ".agent", "ledger", "tasks")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, ".agent", "ledger", "tasks")); err != nil {
		t.Fatal(err)
	}
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = module.InspectLegacyNative(context.Background())
	if code := continuity.ErrorCode(err); code != continuity.CodeUnsafeReference {
		t.Fatalf("error=%v code=%q, want %q", err, code, continuity.CodeUnsafeReference)
	}
}

func TestLegacyMigrationRejectsMoreThanOneTransactionCanHold(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 255)
	module, err := continuity.NewFile(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = module.InspectLegacyNative(context.Background())
	if code := continuity.ErrorCode(err); code != continuity.CodeMigrationTooLarge {
		t.Fatalf("error=%v code=%q, want %q", err, code, continuity.CodeMigrationTooLarge)
	}
}

func TestLegacyMigrationRollbackIsPermanentlyRejectedAfterSave(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 2)
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := previewLegacyMigration(t, kernel)
	applyLegacyMigration(t, kernel, preview, "post-save")
	resumeView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatal(err)
	}
	resume := resumeView.(engine.ResumeView).Capsule
	next := []string{"保留 v2"}
	savePayload, _ := json.Marshal(engine.SaveObjective{ObjectiveID: resume.ObjectiveID, ExpectedObjectiveRevision: resume.Revision, Done: []string{"迁移后继续工作"}, Next: &next})
	saved, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-after-migration", IdempotencyKey: "after-migration", CorrelationID: "after-migration",
		ProjectID: resume.ProjectID, ExpectedRevision: resume.LedgerRevision,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-after", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC), Kind: engine.CommandSaveObjective, Payload: savePayload,
	})
	if err != nil || !saved.Accepted || saved.NewRevision != 2 {
		t.Fatalf("save receipt=%#v err=%v", saved, err)
	}
	_, err = kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
	if code := continuity.ErrorCode(err); code != continuity.CodeRollbackNotAllowed {
		t.Fatalf("rollback query error=%v code=%q, want %q", err, code, continuity.CodeRollbackNotAllowed)
	}
}

func TestLegacyMigrationCannotRestartAfterRollbackRuntimeIsDeleted(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 2)
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := previewLegacyMigration(t, kernel)
	applyLegacyMigration(t, kernel, preview, "persistent-rollback-tombstone")
	rollbackView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
	if err != nil {
		t.Fatal(err)
	}
	rollback := rollbackView.(engine.LegacyRollbackView).Rollback
	payload, _ := json.Marshal(engine.RollbackLegacyMigration{
		MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
	})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-persistent-rollback", IdempotencyKey: "persistent-rollback",
		CorrelationID: "persistent-rollback", ProjectID: rollback.ProjectID, ExpectedRevision: 1,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-persistent-rollback", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC), Kind: engine.CommandRollbackLegacyMigration, Payload: payload,
	})
	if err != nil || !receipt.Accepted {
		t.Fatalf("rollback receipt=%#v err=%v", receipt, err)
	}
	tombstone := filepath.Join(root, ".agent", "archive", "migrations", rollback.MigrationID, "rollback", "started.json")
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("persistent rollback tombstone: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".agent", "runtime")); err != nil {
		t.Fatal(err)
	}
	reopened, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reopened.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration})
	if code := continuity.ErrorCode(err); code != continuity.CodeMigrationNotApplicable {
		t.Fatalf("re-migration query error=%v code=%q, want %q", err, code, continuity.CodeMigrationNotApplicable)
	}
	if _, err := os.Lstat(filepath.Join(root, ".agent", "ledger", "HEAD")); !os.IsNotExist(err) {
		t.Fatalf("re-migration rejection created a live HEAD: %v", err)
	}
}

func TestLegacyRollbackRejectsBackupRewrittenWithItsManifest(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 2)
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := previewLegacyMigration(t, kernel)
	applyLegacyMigration(t, kernel, preview, "tampered-backup")

	backupRoot := filepath.Join(root, ".agent", "archive", "migrations", preview.MigrationID, "v1")
	handoffPath := filepath.Join(backupRoot, "HANDOFF.md")
	tampered := append(mustRead(t, handoffPath), []byte("\n# rewritten backup\n")...)
	if err := os.WriteFile(handoffPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(backupRoot, "manifest.json")
	var manifest struct {
		Schema         string `json:"schema"`
		MigrationID    string `json:"migration_id"`
		ProjectID      string `json:"project_id"`
		SourceDigest   string `json:"source_digest"`
		SemanticDigest string `json:"semantic_digest"`
		Files          []struct {
			Path   string `json:"path"`
			Digest string `json:"digest"`
			Bytes  int    `json:"bytes"`
		} `json:"files"`
	}
	if err := json.Unmarshal(mustRead(t, manifestPath), &manifest); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Files {
		if manifest.Files[index].Path == ".agent/HANDOFF.md" {
			manifest.Files[index].Bytes = len(tampered)
			manifest.Files[index].Digest = fmt.Sprintf("%x", sha256.Sum256(tampered))
		}
	}
	manifestRaw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	rollbackView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
	if err != nil {
		t.Fatal(err)
	}
	rollback := rollbackView.(engine.LegacyRollbackView).Rollback
	payload, _ := json.Marshal(engine.RollbackLegacyMigration{
		MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
	})
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-tampered-rollback", IdempotencyKey: "tampered-rollback",
		CorrelationID: "tampered-rollback", ProjectID: rollback.ProjectID, ExpectedRevision: 1,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-tampered", Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC), Kind: engine.CommandRollbackLegacyMigration, Payload: payload,
	})
	if err != nil || receipt.Accepted || receipt.Rejection == nil || receipt.Rejection.Code != string(continuity.CodeRollbackSourceDrift) {
		t.Fatalf("tampered rollback receipt=%#v err=%v", receipt, err)
	}
}

func TestLegacyRollbackRetryCompletesInterruptedStagesThroughEngine(t *testing.T) {
	for _, test := range []struct {
		name                string
		failAfterQuarantine bool
		failBeforeComplete  bool
	}{
		{name: "ledger quarantined before handoff restore", failAfterQuarantine: true},
		{name: "handoff restored before journal complete", failBeforeComplete: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeLegacyMigrationFixture(t, root, 2)
			originalHandoff := mustRead(t, filepath.Join(root, ".agent", "HANDOFF.md"))
			module, err := continuity.NewFile(root)
			if err != nil {
				t.Fatal(err)
			}
			fileStore, err := ledger.NewFile(filepath.Join(root, ".agent", "ledger"))
			if err != nil {
				t.Fatal(err)
			}
			store := &interruptRollbackStore{
				File: fileStore, failAfterQuarantine: test.failAfterQuarantine, failBeforeComplete: test.failBeforeComplete,
			}
			kernel := engine.New(store, engine.WithContinuity(module))
			preview, _ := previewLegacyMigration(t, kernel)
			applyLegacyMigration(t, kernel, preview, "rollback-interrupt")
			rollbackView, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyRollback})
			if err != nil {
				t.Fatal(err)
			}
			rollback := rollbackView.(engine.LegacyRollbackView).Rollback
			payload, _ := json.Marshal(engine.RollbackLegacyMigration{
				MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
			})
			command := engine.CommandEnvelope{
				Schema: engine.CommandSchemaV2, CommandID: "cmd-interrupted-rollback", IdempotencyKey: "interrupted-rollback",
				CorrelationID: "interrupted-rollback", ProjectID: rollback.ProjectID, ExpectedRevision: 1,
				Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-interrupted", Runtime: "go-test", Role: engine.ActorUser},
				IssuedAt: time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC), Kind: engine.CommandRollbackLegacyMigration, Payload: payload,
			}
			first, err := kernel.Apply(context.Background(), command)
			if err != nil || first.Accepted {
				t.Fatalf("interrupted rollback receipt=%#v err=%v", first, err)
			}
			command.CommandID = "cmd-interrupted-rollback-retry"
			command.IdempotencyKey = "interrupted-rollback-retry"
			command.CorrelationID = "interrupted-rollback-retry"
			retried, err := kernel.Apply(context.Background(), command)
			if err != nil || !retried.Accepted || retried.EntityStatus != "rolled_back" {
				t.Fatalf("rollback retry receipt=%#v err=%v", retried, err)
			}
			if got := mustRead(t, filepath.Join(root, ".agent", "HANDOFF.md")); !bytes.Equal(got, originalHandoff) {
				t.Fatal("rollback retry did not restore the original handoff")
			}
			var journal struct {
				Stage string `json:"stage"`
			}
			if err := json.Unmarshal(mustRead(t, filepath.Join(root, ".agent", "runtime", "migration.rollback.json")), &journal); err != nil || journal.Stage != "complete" {
				t.Fatalf("rollback journal=%#v err=%v", journal, err)
			}
		})
	}
}

func TestConcurrentLegacyMigrationCreatesOneGenesis(t *testing.T) {
	root := t.TempDir()
	writeLegacyMigrationFixture(t, root, 2)
	first, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := previewLegacyMigration(t, first)
	payload, _ := json.Marshal(engine.ImportLegacyNative{MigrationID: preview.MigrationID, SourceDigest: preview.SourceDigest, BackupManifestDigest: preview.BackupManifestDigest})
	commands := []engine.CommandEnvelope{
		migrationCommand(preview.ProjectID, payload, "concurrent-a"),
		migrationCommand(preview.ProjectID, payload, "concurrent-b"),
	}
	kernels := []engine.Engine{first, second}
	type result struct {
		receipt engine.Receipt
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for index := range kernels {
		go func(index int) {
			<-start
			receipt, applyErr := kernels[index].Apply(context.Background(), commands[index])
			results <- result{receipt: receipt, err: applyErr}
		}(index)
	}
	close(start)
	for range 2 {
		got := <-results
		if got.err != nil || !got.receipt.Accepted || got.receipt.NewRevision != 1 {
			t.Fatalf("concurrent migration receipt=%#v err=%v", got.receipt, got.err)
		}
	}
	committedView, err := first.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration, ProjectID: preview.ProjectID})
	if err != nil || committedView.(engine.LegacyMigrationView).LedgerRevision != 1 {
		t.Fatalf("committed migration=%#v err=%v", committedView, err)
	}
}

func previewLegacyMigration(t *testing.T, kernel engine.Engine) (continuity.LegacyMigration, engine.LegacyMigrationView) {
	t.Helper()
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryLegacyMigration})
	if err != nil {
		t.Fatal(err)
	}
	preview := view.(engine.LegacyMigrationView)
	return preview.Migration, preview
}

func applyLegacyMigration(t *testing.T, kernel engine.Engine, plan continuity.LegacyMigration, suffix string) engine.Receipt {
	t.Helper()
	payload, _ := json.Marshal(engine.ImportLegacyNative{MigrationID: plan.MigrationID, SourceDigest: plan.SourceDigest, BackupManifestDigest: plan.BackupManifestDigest})
	receipt, err := kernel.Apply(context.Background(), migrationCommand(plan.ProjectID, payload, suffix))
	if err != nil || !receipt.Accepted {
		t.Fatalf("migration receipt=%#v err=%v", receipt, err)
	}
	return receipt
}

func migrationCommand(projectID string, payload []byte, suffix string) engine.CommandEnvelope {
	return engine.CommandEnvelope{
		Schema: engine.CommandSchemaV2, CommandID: "cmd-" + suffix, IdempotencyKey: "migrate-" + suffix, CorrelationID: "migrate-" + suffix,
		ProjectID: projectID, ExpectedRevision: 0,
		Actor:    engine.ActorRef{ActorID: "user-fixture", SessionID: "session-" + suffix, Runtime: "go-test", Role: engine.ActorUser},
		IssuedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), Kind: engine.CommandImportLegacyNative, Payload: payload,
	}
}

type legacyFixture struct {
	activeGoal      string
	completedTaskID string
}

type interruptRollbackStore struct {
	*ledger.File
	failAfterQuarantine bool
	failBeforeComplete  bool
}

func (s *interruptRollbackStore) QuarantineGenesis(ctx context.Context, ref ledger.GenesisRef, migrationID string) error {
	if err := s.File.QuarantineGenesis(ctx, ref, migrationID); err != nil {
		return err
	}
	if s.failAfterQuarantine {
		s.failAfterQuarantine = false
		return errors.New("injected interruption after ledger quarantine")
	}
	return nil
}

func (s *interruptRollbackStore) CompleteGenesisQuarantine(ctx context.Context, ref ledger.GenesisRef, migrationID string) error {
	if s.failBeforeComplete {
		s.failBeforeComplete = false
		return errors.New("injected interruption before rollback journal completion")
	}
	return s.File.CompleteGenesisQuarantine(ctx, ref, migrationID)
}

func writeLegacyMigrationFixture(t *testing.T, root string, factCount int) legacyFixture {
	t.Helper()
	writeFile(t, filepath.Join(root, "README.md"), []byte("# fixture\n"))
	completedID := "task_completed"
	activeID := "task_active"
	activeGoal := "迁移全部 v1 记忆"

	completed := legacyTaskMap(completedID, "已完成目标", "completed", 5)
	completed["review"] = map[string]any{
		"approved": true, "findings": []string{}, "reviewed_revision": 5, "summary": "独立审查通过",
		"reviewed_at": "2026-07-14T10:00:00Z", "reviewed_by": "reviewer-fixture", "independent": true,
	}
	completed["closeout"] = map[string]any{
		"summary": "历史目标完成", "completed_at": "2026-07-14T10:01:00Z", "completed_by": "user-fixture", "residual_acknowledged": true,
	}
	active := legacyTaskMap(activeID, activeGoal, "active", 3)
	active["must_read"] = []string{"README.md"}
	active["next"] = []string{"继续 v2 工作"}
	active["done"] = []string{"完成 dry-run 设计"}
	active["validation"] = []string{"fixture validation"}

	completedRaw := renderLegacyDocument(t, completed, "历史任务", taskSections(completed))
	activeRaw := renderLegacyDocument(t, active, "当前任务", taskSections(active))
	writeFile(t, filepath.Join(root, ".agent", "ledger", "tasks", completedID+".md"), completedRaw)
	activePath := filepath.ToSlash(filepath.Join(".agent", "ledger", "tasks", activeID+".md"))
	writeFile(t, filepath.Join(root, filepath.FromSlash(activePath)), activeRaw)

	for index := 0; index < 4; index++ {
		taskID := activeID
		if index == 0 {
			taskID = completedID
		}
		decision := map[string]any{
			"schema": "summer-harness/v1", "kind": "decision", "id": fmt.Sprintf("dec_%02d", index), "task_id": taskID,
			"title": fmt.Sprintf("Decision %02d", index), "question": "如何迁移？", "chosen": "单事务导入",
			"rejected": []string{"截断"}, "why_not": []string{"会丢记忆"}, "source": "fixture",
			"created_at": fmt.Sprintf("2026-07-14T11:%02d:00Z", index), "created_ns": int64(index + 1), "created_by": "session-fixture",
		}
		raw := renderLegacyDocument(t, decision, decision["title"].(string), []legacySection{
			{"问题", []string{decision["question"].(string)}}, {"选择", []string{decision["chosen"].(string)}},
			{"拒绝", []string{"截断"}}, {"为什么不选", []string{"会丢记忆"}}, {"来源", []string{"fixture"}},
		})
		writeFile(t, filepath.Join(root, ".agent", "ledger", "decisions", decision["id"].(string)+".md"), raw)
	}

	var facts bytes.Buffer
	for index := 0; index < factCount; index++ {
		fact := map[string]any{
			"schema": "summer-harness/v1", "kind": "fact", "id": fmt.Sprintf("fact_%03d", index), "task_id": activeID,
			"statement": fmt.Sprintf("观察 %03d", index), "source": "fixture", "confidence": "high", "tags": []string{"migration"},
			"memory_class": "durable", "observed_at": "2026-07-14T12:00:00Z", "created_ns": int64(index + 1), "session": "session-fixture",
		}
		line, _ := json.Marshal(fact)
		facts.Write(line)
		facts.WriteByte('\n')
		if index == 0 {
			invalidation := map[string]any{
				"schema": "summer-harness/v1", "kind": "fact_invalidation", "id": "inv_000", "task_id": activeID,
				"invalidates": "fact_000", "reason": "由更新事实替代", "observed_at": "2026-07-14T12:01:00Z", "created_ns": int64(2), "session": "session-fixture",
			}
			line, _ = json.Marshal(invalidation)
			facts.Write(line)
			facts.WriteByte('\n')
		}
	}
	writeFile(t, filepath.Join(root, ".agent", "ledger", "facts", activeID+".jsonl"), facts.Bytes())

	activeDigest := fmt.Sprintf("%x", sha256.Sum256(activeRaw))
	handoff := map[string]any{
		"schema": "summer-harness/v1", "mode": "native", "engine": "summer", "goal": activeGoal,
		"done": active["done"], "next": active["next"], "validation": active["validation"], "blockers": []string{}, "must_read": active["must_read"],
		"source_path": activePath, "source_digest": activeDigest, "task_id": activeID, "task_status": "active",
		"resume_command": "$project-handoff", "updated_at": "2026-07-14T12:02:00Z", "last_writer": "session-fixture",
	}
	handoffRaw := renderLegacyDocument(t, handoff, "Project Handoff", []legacySection{
		{"当前目标", []string{activeGoal}}, {"已完成", active["done"].([]string)}, {"唯一下一步", active["next"].([]string)},
		{"验证", active["validation"].([]string)}, {"阻塞", []string{}}, {"必须读取", active["must_read"].([]string)},
	})
	writeFile(t, filepath.Join(root, ".agent", "HANDOFF.md"), handoffRaw)
	return legacyFixture{activeGoal: activeGoal, completedTaskID: completedID}
}

func legacyTaskMap(id, goal, status string, revision uint64) map[string]any {
	title := "当前任务"
	if status == "completed" {
		title = "历史任务"
	}
	return map[string]any{
		"schema": "summer-harness/v1", "kind": "task", "id": id, "title": title, "goal": goal,
		"acceptance": []string{"完整迁移"}, "status": status, "profile": "high-risk", "risk": "high",
		"revision": revision, "validation_revision": revision, "done": []string{}, "next": []string{}, "validation": []string{},
		"blockers": []string{}, "must_read": []string{}, "residual_risks": []string{}, "engine": "summer",
		"created_at": "2026-07-14T09:00:00Z", "created_by": "session-fixture", "updated_at": "2026-07-14T09:01:00Z",
		"last_writer": "session-fixture", "last_work_session": "session-fixture",
		"review": map[string]any{"approved": false, "findings": []string{}, "reviewed_revision": uint64(0), "summary": ""},
	}
}

func taskSections(task map[string]any) []legacySection {
	return []legacySection{
		{"目标", []string{task["goal"].(string)}}, {"验收条件", task["acceptance"].([]string)}, {"已完成", task["done"].([]string)},
		{"下一步", task["next"].([]string)}, {"验证", task["validation"].([]string)}, {"阻塞", task["blockers"].([]string)},
		{"必须读取", task["must_read"].([]string)}, {"残余风险", task["residual_risks"].([]string)},
	}
}

type legacySection struct {
	heading string
	values  []string
}

func renderLegacyDocument(t *testing.T, meta map[string]any, title string, sections []legacySection) []byte {
	t.Helper()
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	output.WriteString("---\n")
	output.Write(encoded)
	output.WriteString("\n---\n# ")
	output.WriteString(title)
	output.WriteString("\n\n")
	for _, section := range sections {
		if len(section.values) == 0 {
			continue
		}
		output.WriteString("## ")
		output.WriteString(section.heading)
		output.WriteString("\n\n")
		for _, value := range section.values {
			output.WriteString("- ")
			output.WriteString(value)
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n")
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
