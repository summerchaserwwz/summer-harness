package ledger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

func TestFileLedgerSerializesWritersAcrossInstances(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const writers = 32
	stores := make([]*ledger.File, writers)
	for index := range stores {
		store, err := ledger.NewFile(root)
		if err != nil {
			t.Fatalf("new file ledger %d: %v", index, err)
		}
		stores[index] = store
	}

	start := make(chan struct{})
	errorsByWriter := make([]error, writers)
	var wait sync.WaitGroup
	wait.Add(writers)
	for index, store := range stores {
		go func(index int, store *ledger.File) {
			defer wait.Done()
			<-start
			_, errorsByWriter[index] = store.Commit(context.Background(), testDraft(index), 0)
		}(index, store)
	}
	close(start)
	wait.Wait()

	succeeded := 0
	for index, err := range errorsByWriter {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ledger.ErrRevisionConflict):
		default:
			t.Fatalf("writer %d returned unexpected error: %v", index, err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful writers = %d, want 1", succeeded)
	}

	transactions, err := stores[0].Transactions(context.Background(), "project-concurrent")
	if err != nil {
		t.Fatalf("read transactions after concurrent commits: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(transactions))
	}
}

func TestFileLedgerSerializesWritersAcrossProcesses(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	startPath := filepath.Join(root, "start")
	commands := make([]*exec.Cmd, 2)
	results := make([]string, 2)
	for index := range commands {
		resultPath := filepath.Join(root, fmt.Sprintf("result-%d", index))
		results[index] = resultPath
		command := exec.Command(os.Args[0], "-test.run=^TestFileLedgerProcessWriter$")
		command.Env = append(os.Environ(),
			"SUMMER_LEDGER_PROCESS_WRITER=1",
			"SUMMER_LEDGER_ROOT="+filepath.Join(root, "ledger"),
			"SUMMER_LEDGER_START="+startPath,
			"SUMMER_LEDGER_RESULT="+resultPath,
			fmt.Sprintf("SUMMER_LEDGER_INDEX=%d", index),
		)
		if err := command.Start(); err != nil {
			t.Fatalf("start writer %d: %v", index, err)
		}
		commands[index] = command
	}
	if err := os.WriteFile(startPath, []byte("go\n"), 0o600); err != nil {
		t.Fatalf("release process writers: %v", err)
	}
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("wait writer %d: %v", index, err)
		}
	}

	counts := map[string]int{}
	for index, resultPath := range results {
		raw, err := os.ReadFile(resultPath)
		if err != nil {
			t.Fatalf("read writer %d result: %v", index, err)
		}
		counts[string(bytes.TrimSpace(raw))]++
	}
	if counts["committed"] != 1 || counts["revision_conflict"] != 1 {
		t.Fatalf("process writer results = %#v, want one commit and one revision conflict", counts)
	}
}

func TestFileLedgerProcessWriter(t *testing.T) {
	if os.Getenv("SUMMER_LEDGER_PROCESS_WRITER") != "1" {
		return
	}
	root := os.Getenv("SUMMER_LEDGER_ROOT")
	startPath := os.Getenv("SUMMER_LEDGER_START")
	resultPath := os.Getenv("SUMMER_LEDGER_RESULT")
	index := 0
	if _, err := fmt.Sscanf(os.Getenv("SUMMER_LEDGER_INDEX"), "%d", &index); err != nil {
		t.Fatalf("parse writer index: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(startPath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inspect writer start signal: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for writer start signal")
		}
		time.Sleep(5 * time.Millisecond)
	}
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new process file ledger: %v", err)
	}
	result := "unexpected"
	if _, err := store.Commit(context.Background(), testDraft(index), 0); err == nil {
		result = "committed"
	} else if errors.Is(err, ledger.ErrRevisionConflict) {
		result = "revision_conflict"
	} else {
		t.Fatalf("process commit: %v", err)
	}
	if err := os.WriteFile(resultPath, []byte(result+"\n"), 0o600); err != nil {
		t.Fatalf("write process result: %v", err)
	}
}

func TestFileLedgerAdoptsDurableTransactionWhenHeadUpdateFailed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatalf("make ledger root read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := store.Commit(context.Background(), testDraft(0), 0); err == nil {
		t.Fatal("commit succeeded even though HEAD could not be written")
	}
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("restore ledger root permissions: %v", err)
	}

	transactions, err := store.Transactions(context.Background(), "project-concurrent")
	if err != nil {
		t.Fatalf("recover durable orphan transaction: %v", err)
	}
	if len(transactions) != 1 || transactions[0].Revision != 1 {
		t.Fatalf("recovered transactions = %#v, want one revision-1 transaction", transactions)
	}
	head, err := store.Head(context.Background(), "project-concurrent")
	if err != nil {
		t.Fatalf("read recovered HEAD: %v", err)
	}
	if head.Revision != 1 || head.Digest != transactions[0].Digest {
		t.Fatalf("recovered HEAD = %#v, transaction = %#v", head, transactions[0])
	}
}

func TestFileLedgerDoesNotAdoptOrphanWithoutLocalRecoveryMarker(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	source, err := ledger.NewFile(sourceRoot)
	if err != nil {
		t.Fatalf("new source ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := source.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit source transaction: %v", err)
	}

	targetRoot := t.TempDir()
	target, err := ledger.NewFile(targetRoot)
	if err != nil {
		t.Fatalf("new target ledger: %v", err)
	}
	sourceDirectory := filepath.Join(sourceRoot, "transactions", draft.TransactionID)
	targetDirectory := filepath.Join(targetRoot, "transactions", draft.TransactionID)
	if err := os.Mkdir(targetDirectory, 0o755); err != nil {
		t.Fatalf("create injected transaction directory: %v", err)
	}
	for _, name := range []string{"0001.json", "manifest.json"} {
		raw, err := os.ReadFile(filepath.Join(sourceDirectory, name))
		if err != nil {
			t.Fatalf("read source %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(targetDirectory, name), raw, 0o644); err != nil {
			t.Fatalf("write injected %s: %v", name, err)
		}
	}

	if _, err := target.Transactions(context.Background(), draft.ProjectID); err == nil {
		t.Fatal("ledger adopted an orphan transaction without a local recovery marker")
	}
}

func TestFileLedgerRejectsTransactionIDOutsideLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	draft.TransactionID = "../escaped"
	if _, err := store.Commit(context.Background(), draft, 0); err == nil {
		t.Fatal("commit accepted transaction id outside transactions directory")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped transaction path exists or cannot be inspected: %v", err)
	}
}

func TestFileLedgerRejectsSymlinkEventFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := store.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	eventPath := filepath.Join(root, "transactions", draft.TransactionID, "0001.json")
	eventRaw, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read committed event: %v", err)
	}
	outside := filepath.Join(root, "outside-event.json")
	if err := os.WriteFile(outside, eventRaw, 0o644); err != nil {
		t.Fatalf("write outside event: %v", err)
	}
	if err := os.Remove(eventPath); err != nil {
		t.Fatalf("remove committed event: %v", err)
	}
	if err := os.Symlink(outside, eventPath); err != nil {
		t.Fatalf("replace event with symlink: %v", err)
	}

	if _, err := store.Transactions(context.Background(), "project-concurrent"); err == nil {
		t.Fatal("ledger accepted a symlink event file")
	}
}

func TestFileLedgerRejectsTraversalInManifestEventPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := store.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	transactionDirectory := filepath.Join(root, "transactions", draft.TransactionID)
	eventRaw, err := os.ReadFile(filepath.Join(transactionDirectory, "0001.json"))
	if err != nil {
		t.Fatalf("read committed event: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside-event.json"), eventRaw, 0o644); err != nil {
		t.Fatalf("write outside event: %v", err)
	}
	manifestPath := filepath.Join(transactionDirectory, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["event_files"] = []string{"../../outside-event.json"}
	manifestRaw, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered manifest: %v", err)
	}

	if _, err := store.Transactions(context.Background(), draft.ProjectID); err == nil {
		t.Fatal("ledger accepted traversal in manifest event path")
	}
}

func TestFileLedgerRejectsUnknownManifestFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := store.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	manifestPath := filepath.Join(root, "transactions", draft.TransactionID, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["unsigned_extension"] = "must not be ignored"
	manifestRaw, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, append(manifestRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered manifest: %v", err)
	}

	if _, err := store.Transactions(context.Background(), draft.ProjectID); err == nil {
		t.Fatal("ledger ignored an unknown manifest field outside the transaction digest")
	}
}

func TestFileLedgerPersistsCommandProvenance(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	draft.CorrelationID = "corr-provenance"
	draft.CausationID = "evt-parent"
	draft.IssuedAt = time.Date(2026, 7, 15, 9, 30, 0, 0, time.UTC)
	committed, err := store.Commit(context.Background(), draft, 0)
	if err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	if committed.CommittedAt.IsZero() {
		t.Fatal("committed transaction has no committed_at")
	}

	reopened, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("reopen file ledger: %v", err)
	}
	transactions, err := reopened.Transactions(context.Background(), draft.ProjectID)
	if err != nil {
		t.Fatalf("read persisted transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(transactions))
	}
	got := transactions[0]
	if got.CorrelationID != draft.CorrelationID || got.CausationID != draft.CausationID || !got.IssuedAt.Equal(draft.IssuedAt) || !got.CommittedAt.Equal(committed.CommittedAt) {
		t.Fatalf("persisted provenance = %#v, committed = %#v", got, committed)
	}
}

func TestLedgerStoresRejectTransactionWithoutEvents(t *testing.T) {
	t.Parallel()

	fileStore, err := ledger.NewFile(t.TempDir())
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	stores := map[string]ledger.Store{
		"memory": ledger.NewMemory(),
		"file":   fileStore,
	}
	for name, store := range stores {
		name, store := name, store
		t.Run(name, func(t *testing.T) {
			draft := testDraft(0)
			draft.Events = nil
			if _, err := store.Commit(context.Background(), draft, 0); err == nil {
				t.Fatal("commit accepted transaction without events")
			}
			head, err := store.Head(context.Background(), draft.ProjectID)
			if err != nil {
				t.Fatalf("read head after rejected transaction: %v", err)
			}
			if head.Revision != 0 {
				t.Fatalf("head revision = %d, want 0", head.Revision)
			}
		})
	}
}

func TestLedgerStoresEnforceIdempotencyInsideCommit(t *testing.T) {
	t.Parallel()

	fileStore, err := ledger.NewFile(t.TempDir())
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	stores := map[string]ledger.Store{
		"memory": ledger.NewMemory(),
		"file":   fileStore,
	}
	for name, store := range stores {
		name, store := name, store
		t.Run(name, func(t *testing.T) {
			firstDraft := testDraft(0)
			first, err := store.Commit(context.Background(), firstDraft, 0)
			if err != nil {
				t.Fatalf("first commit: %v", err)
			}
			retryDraft := firstDraft
			retryDraft.TransactionID = "tx-idempotent-retry"
			retry, err := store.Commit(context.Background(), retryDraft, 1)
			if err != nil {
				t.Fatalf("idempotent commit retry: %v", err)
			}
			if retry.TransactionID != first.TransactionID || retry.Revision != 1 {
				t.Fatalf("idempotent retry = %#v, want original %#v", retry, first)
			}
			conflictDraft := retryDraft
			conflictDraft.TransactionID = "tx-idempotent-conflict"
			conflictDraft.CommandDigest = "different-command-digest"
			if _, err := store.Commit(context.Background(), conflictDraft, 1); !errors.Is(err, ledger.ErrIdempotencyConflict) {
				t.Fatalf("conflicting idempotency error = %v, want ErrIdempotencyConflict", err)
			}
			head, err := store.Head(context.Background(), firstDraft.ProjectID)
			if err != nil {
				t.Fatalf("read head: %v", err)
			}
			if head.Revision != 1 {
				t.Fatalf("head revision = %d after retries, want 1", head.Revision)
			}
		})
	}
}

func TestLedgerStoresRejectDuplicateCanonicalIDs(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		mutate    func(first, second *ledger.Draft)
		wantError error
	}{
		{
			name: "transaction id",
			mutate: func(first, second *ledger.Draft) {
				second.TransactionID = first.TransactionID
			},
			wantError: ledger.ErrTransactionIDConflict,
		},
		{
			name: "command id",
			mutate: func(first, second *ledger.Draft) {
				second.CommandID = first.CommandID
			},
			wantError: ledger.ErrCommandIDConflict,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			for storeName, newStore := range storeFactories(t) {
				storeName, newStore := storeName, newStore
				t.Run(storeName, func(t *testing.T) {
					store := newStore()
					first := testDraft(0)
					if _, err := store.Commit(context.Background(), first, 0); err != nil {
						t.Fatalf("first commit: %v", err)
					}
					second := testDraft(1)
					testCase.mutate(&first, &second)
					if _, err := store.Commit(context.Background(), second, 1); !errors.Is(err, testCase.wantError) {
						t.Fatalf("duplicate id error = %v, want %v", err, testCase.wantError)
					}
					head, err := store.Head(context.Background(), first.ProjectID)
					if err != nil {
						t.Fatalf("read head after rejected duplicate: %v", err)
					}
					if head.Revision != 1 {
						t.Fatalf("head revision = %d, want 1", head.Revision)
					}
				})
			}
		})
	}
}

func TestLedgerStoresRejectSecondProject(t *testing.T) {
	t.Parallel()

	for storeName, newStore := range storeFactories(t) {
		storeName, newStore := storeName, newStore
		t.Run(storeName, func(t *testing.T) {
			store := newStore()
			first := testDraft(0)
			if _, err := store.Commit(context.Background(), first, 0); err != nil {
				t.Fatalf("first commit: %v", err)
			}
			if _, err := store.Head(context.Background(), "another-project"); !errors.Is(err, ledger.ErrProjectConflict) {
				t.Fatalf("second project error = %v, want ErrProjectConflict", err)
			}
		})
	}
}

func TestFileLedgerClearsTruncatedPendingMarkerWithoutOrphan(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	pending := filepath.Join(filepath.Dir(root), "runtime", "ledger.pending.json")
	if err := os.WriteFile(pending, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatalf("write truncated pending marker: %v", err)
	}
	transactions, err := store.Transactions(context.Background(), "project-concurrent")
	if err != nil {
		t.Fatalf("recover truncated marker without orphan: %v", err)
	}
	if len(transactions) != 0 {
		t.Fatalf("transactions = %d, want 0", len(transactions))
	}
	if _, err := os.Stat(pending); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("truncated pending marker still exists: %v", err)
	}
}

func TestFileLedgerRejectsDuplicateManifestKeys(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := ledger.NewFile(root)
	if err != nil {
		t.Fatalf("new file ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := store.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	manifestPath := filepath.Join(root, "transactions", draft.TransactionID, "manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	needle := []byte(`  "project_id": "project-concurrent",`)
	replacement := append(append([]byte{}, needle...), append([]byte("\n"), needle...)...)
	raw = bytes.Replace(raw, needle, replacement, 1)
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatalf("write duplicate-key manifest: %v", err)
	}
	if _, err := store.Transactions(context.Background(), draft.ProjectID); err == nil {
		t.Fatal("ledger accepted duplicate manifest keys")
	}
}

func TestFileLedgerRejectsCorruptRecoveryMarkerForOrphan(t *testing.T) {
	t.Parallel()

	sourceRoot := t.TempDir()
	source, err := ledger.NewFile(sourceRoot)
	if err != nil {
		t.Fatalf("new source ledger: %v", err)
	}
	draft := testDraft(0)
	if _, err := source.Commit(context.Background(), draft, 0); err != nil {
		t.Fatalf("commit source transaction: %v", err)
	}

	targetRoot := t.TempDir()
	target, err := ledger.NewFile(targetRoot)
	if err != nil {
		t.Fatalf("new target ledger: %v", err)
	}
	sourceDirectory := filepath.Join(sourceRoot, "transactions", draft.TransactionID)
	targetDirectory := filepath.Join(targetRoot, "transactions", draft.TransactionID)
	if err := os.Mkdir(targetDirectory, 0o755); err != nil {
		t.Fatalf("create orphan directory: %v", err)
	}
	for _, name := range []string{"0001.json", "manifest.json"} {
		raw, err := os.ReadFile(filepath.Join(sourceDirectory, name))
		if err != nil {
			t.Fatalf("read source %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(targetDirectory, name), raw, 0o644); err != nil {
			t.Fatalf("write orphan %s: %v", name, err)
		}
	}
	pending := filepath.Join(filepath.Dir(targetRoot), "runtime", "ledger.pending.json")
	if err := os.WriteFile(pending, []byte(`{"schema":`), 0o600); err != nil {
		t.Fatalf("write corrupt recovery marker: %v", err)
	}
	if _, err := target.Transactions(context.Background(), draft.ProjectID); err == nil {
		t.Fatal("ledger adopted an orphan with a corrupt recovery marker")
	}
}

func storeFactories(t *testing.T) map[string]func() ledger.Store {
	t.Helper()
	return map[string]func() ledger.Store{
		"memory": func() ledger.Store { return ledger.NewMemory() },
		"file": func() ledger.Store {
			store, err := ledger.NewFile(t.TempDir())
			if err != nil {
				t.Fatalf("new file ledger: %v", err)
			}
			return store
		},
	}
}

func testDraft(index int) ledger.Draft {
	data, err := json.Marshal(map[string]int{"writer": index})
	if err != nil {
		panic(err)
	}
	return ledger.Draft{
		TransactionID:  fmt.Sprintf("tx-writer-%02d", index),
		ProjectID:      "project-concurrent",
		CommandID:      fmt.Sprintf("cmd-writer-%02d", index),
		CommandDigest:  fmt.Sprintf("digest-writer-%02d", index),
		IdempotencyKey: fmt.Sprintf("writer-%02d", index),
		CorrelationID:  fmt.Sprintf("corr-writer-%02d", index),
		IssuedAt:       time.Date(2026, 7, 15, 11, index%60, 0, 0, time.UTC),
		Actor:          json.RawMessage(`{"actor_id":"test"}`),
		Events: []ledger.Event{{
			EventID:  fmt.Sprintf("evt-writer-%02d", index),
			Kind:     "TestEvent",
			EntityID: "entity-concurrent",
			Data:     data,
		}},
	}
}

func TestFileLedgerRejectsSymlinkRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real-ledger")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatalf("create real ledger root: %v", err)
	}
	linkedRoot := filepath.Join(parent, "linked-ledger")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatalf("create ledger symlink: %v", err)
	}

	if _, err := ledger.NewFile(linkedRoot); err == nil {
		t.Fatal("NewFile accepted a symlink ledger root")
	}
}

func TestFileLedgerRejectsSymlinkAncestor(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	realParent := filepath.Join(parent, "real-parent")
	if err := os.Mkdir(realParent, 0o755); err != nil {
		t.Fatalf("create real parent: %v", err)
	}
	linkedParent := filepath.Join(parent, "linked-parent")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatalf("create parent symlink: %v", err)
	}
	if _, err := ledger.NewFile(filepath.Join(linkedParent, "ledger")); err == nil {
		t.Fatal("NewFile accepted a ledger path through a symlink ancestor")
	}
}

func TestFileLedgerRejectsSymlinkAncestorWhenLedgerAlreadyExists(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	realParent := filepath.Join(parent, "real-parent")
	if err := os.MkdirAll(filepath.Join(realParent, "ledger"), 0o755); err != nil {
		t.Fatalf("create existing real ledger: %v", err)
	}
	linkedParent := filepath.Join(parent, "linked-parent")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatalf("create parent symlink: %v", err)
	}
	if _, err := ledger.NewFile(filepath.Join(linkedParent, "ledger")); err == nil {
		t.Fatal("NewFile accepted an existing ledger through a symlink ancestor")
	}
}
