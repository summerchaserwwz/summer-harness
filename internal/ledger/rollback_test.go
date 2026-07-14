package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenesisQuarantineResumesAfterTransactionMove(t *testing.T) {
	agent := filepath.Join(t.TempDir(), ".agent")
	store, err := NewFile(filepath.Join(agent, "ledger"))
	if err != nil {
		t.Fatal(err)
	}
	draft := Draft{
		TransactionID: "tx-migration-genesis", ProjectID: "project-migration", CommandID: "cmd-migration",
		CommandDigest: "migration-digest", IdempotencyKey: "migration", CorrelationID: "migration",
		IssuedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), Actor: json.RawMessage(`{"role":"user"}`),
		Events: []Event{{EventID: "evt-migration", Kind: "LegacyMigrationCompleted", EntityID: "migration_fixture", Data: json.RawMessage(`{"migration_id":"migration_fixture"}`)}},
	}
	transaction, err := store.Commit(context.Background(), draft, 0)
	if err != nil {
		t.Fatal(err)
	}
	ref := GenesisRef{ProjectID: transaction.ProjectID, TransactionID: transaction.TransactionID, Digest: transaction.Digest}
	migrationID := "migration_fixture"
	archive := filepath.Join(agent, "archive", "migrations", migrationID, "rollback", "v2")
	if err := makeRegularDirectories(filepath.Join(archive, "transactions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(store.transactions, transaction.TransactionID), filepath.Join(archive, "transactions", transaction.TransactionID)); err != nil {
		t.Fatal(err)
	}
	journal := rollbackJournal{Schema: "summer.migration-rollback/v1", MigrationID: migrationID, Genesis: ref, Stage: "transaction_quarantined"}
	if err := writeAtomicJSONFile(filepath.Join(store.runtime, "migration.rollback.json"), journal, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.QuarantineGenesis(context.Background(), ref, migrationID); err != nil {
		t.Fatalf("resume quarantine: %v", err)
	}
	if _, err := os.Lstat(store.headPath); !os.IsNotExist(err) {
		t.Fatalf("live HEAD remains after resumed quarantine: %v", err)
	}
	if _, err := os.Stat(filepath.Join(archive, "HEAD")); err != nil {
		t.Fatalf("quarantined HEAD: %v", err)
	}
	if err := store.CompleteGenesisQuarantine(context.Background(), ref, migrationID); err != nil {
		t.Fatalf("complete quarantine: %v", err)
	}
	if err := store.QuarantineGenesis(context.Background(), ref, migrationID); err != nil {
		t.Fatalf("repeat completed quarantine: %v", err)
	}
}
