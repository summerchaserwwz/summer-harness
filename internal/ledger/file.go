package ledger

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var ledgerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

const (
	ledgerHeadSchema        = "summer.ledger-head/v2"
	transactionSchema       = "summer.transaction/v2"
	pendingCommitSchema     = "summer.pending-commit/v2"
	rollbackTombstoneSchema = "summer.migration-rollback-started/v1"
)

const (
	maxHeadBytes     = 64 << 10
	maxManifestBytes = 1 << 20
	maxEventBytes    = maxEventDataBytes + (64 << 10)
	maxTransactions  = 100_000
	maxLedgerBytes   = 256 << 20
)

type File struct {
	mu           sync.Mutex
	root         string
	transactions string
	headPath     string
	runtime      string
	lockPath     string
	pendingPath  string
}

type writeLockOwner struct {
	PID       int       `json:"pid"`
	Token     string    `json:"token"`
	Hostname  string    `json:"hostname"`
	CreatedAt time.Time `json:"created_at"`
}

type fileHead struct {
	Schema        string `json:"schema"`
	ProjectID     string `json:"project_id"`
	TransactionID string `json:"transaction_id"`
	Revision      uint64 `json:"revision"`
	Digest        string `json:"digest"`
	ResumeDigest  string `json:"resume_digest,omitempty"`
}

type fileManifest struct {
	Schema         string          `json:"schema"`
	TransactionID  string          `json:"transaction_id"`
	ProjectID      string          `json:"project_id"`
	CommandID      string          `json:"command_id"`
	CommandDigest  string          `json:"command_digest"`
	ResumeDigest   string          `json:"resume_digest,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	CorrelationID  string          `json:"correlation_id"`
	CausationID    string          `json:"causation_id,omitempty"`
	IssuedAt       time.Time       `json:"issued_at"`
	Actor          json.RawMessage `json:"actor"`
	Revision       uint64          `json:"revision"`
	PreviousDigest string          `json:"previous_digest"`
	Digest         string          `json:"digest"`
	CommittedAt    time.Time       `json:"committed_at"`
	EventFiles     []string        `json:"event_files"`
}

type pendingCommit struct {
	Schema         string `json:"schema"`
	ProjectID      string `json:"project_id"`
	TransactionID  string `json:"transaction_id"`
	Revision       uint64 `json:"revision"`
	PreviousDigest string `json:"previous_digest"`
	Digest         string `json:"digest"`
}

type rollbackJournal struct {
	Schema      string     `json:"schema"`
	MigrationID string     `json:"migration_id"`
	Genesis     GenesisRef `json:"genesis"`
	Stage       string     `json:"stage"`
}

type rollbackTombstone struct {
	Schema      string     `json:"schema"`
	MigrationID string     `json:"migration_id"`
	Genesis     GenesisRef `json:"genesis"`
}

func NewFile(root string) (*File, error) {
	root = filepath.Clean(root)
	if root == "." || strings.TrimSpace(root) == "" {
		return nil, errors.New("file ledger root is required")
	}
	absoluteRoot, err := canonicalLedgerRoot(root)
	if err != nil {
		return nil, err
	}
	root = absoluteRoot
	if err := rejectSymlinkDirectory(root); err != nil {
		return nil, err
	}
	transactions := filepath.Join(root, "transactions")
	if err := rejectSymlinkDirectory(transactions); err != nil {
		return nil, err
	}
	runtimeDirectory := filepath.Join(filepath.Dir(root), "runtime")
	if err := rejectSymlinkDirectory(runtimeDirectory); err != nil {
		return nil, err
	}
	createdDirectories, err := missingDirectories(transactions, runtimeDirectory)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(transactions, 0o755); err != nil {
		return nil, fmt.Errorf("create transaction directory: %w", err)
	}
	if err := os.MkdirAll(runtimeDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("create ledger runtime directory: %w", err)
	}
	if err := persistCreatedDirectories(createdDirectories); err != nil {
		return nil, err
	}
	if err := rejectSymlinkDirectory(root); err != nil {
		return nil, err
	}
	if err := rejectSymlinkDirectory(transactions); err != nil {
		return nil, err
	}
	if err := rejectSymlinkDirectory(runtimeDirectory); err != nil {
		return nil, err
	}
	return &File{
		root:         root,
		transactions: transactions,
		headPath:     filepath.Join(root, "HEAD"),
		runtime:      runtimeDirectory,
		lockPath:     filepath.Join(runtimeDirectory, "ledger.write.lock"),
		pendingPath:  filepath.Join(runtimeDirectory, "ledger.pending.json"),
	}, nil
}

func missingDirectories(paths ...string) ([]string, error) {
	missing := make(map[string]struct{})
	for _, path := range paths {
		for current := filepath.Clean(path); ; current = filepath.Dir(current) {
			info, err := os.Lstat(current)
			if err == nil {
				if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
					return nil, fmt.Errorf("ledger path component is not a regular directory: %s", current)
				}
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("inspect ledger path component: %w", err)
			}
			missing[current] = struct{}{}
			parent := filepath.Dir(current)
			if parent == current {
				return nil, errors.New("ledger path has no existing directory ancestor")
			}
		}
	}
	directories := make([]string, 0, len(missing))
	for directory := range missing {
		directories = append(directories, directory)
	}
	sort.Slice(directories, func(i, j int) bool {
		return strings.Count(directories[i], string(os.PathSeparator)) > strings.Count(directories[j], string(os.PathSeparator))
	})
	return directories, nil
}

func persistCreatedDirectories(directories []string) error {
	synced := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		parent := filepath.Dir(directory)
		if _, exists := synced[parent]; exists {
			continue
		}
		if err := fsyncDirectory(parent); err != nil {
			return fmt.Errorf("persist created directory %s: %w", directory, err)
		}
		synced[parent] = struct{}{}
	}
	return nil
}

func canonicalLedgerRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve file ledger root: %w", err)
	}
	if err := rejectLedgerPathSymlinks(absolute); err != nil {
		return "", err
	}
	missing := make([]string, 0, 4)
	for current := filepath.Clean(absolute); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("ledger path cannot pass through a symlink: %s", current)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("ledger path ancestor is not a directory: %s", current)
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("resolve ledger path ancestor: %w", err)
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect ledger path ancestor: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("file ledger root has no existing directory ancestor")
		}
		missing = append(missing, filepath.Base(current))
	}
}

func rejectLedgerPathSymlinks(path string) error {
	checks := 2
	parent := filepath.Dir(path)
	if filepath.Base(path) == "ledger" && filepath.Base(parent) == ".agent" {
		checks = 3
	}
	current := filepath.Clean(path)
	for index := 0; index < checks; index++ {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("ledger path cannot pass through a symlink: %s", current)
			}
			if !info.IsDir() {
				return fmt.Errorf("ledger path component is not a directory: %s", current)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect ledger path component: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func rejectSymlinkDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect ledger path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ledger path cannot be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("ledger path is not a directory: %s", path)
	}
	return nil
}

func (f *File) Project(ctx context.Context) (_ string, _ bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.acquireLedgerLock(ctx, false)
	if err != nil {
		return "", false, err
	}
	defer finishUnlock(unlock, &err)
	head, err := f.readHead()
	if err != nil {
		return "", false, err
	}
	if !isZeroHead(head) {
		if err := validateFileHead(head); err != nil {
			return "", false, err
		}
		return head.ProjectID, true, nil
	}
	pending, found, err := f.readPendingCommit()
	if err != nil {
		return "", false, err
	}
	if found {
		return pending.ProjectID, true, nil
	}
	return "", false, nil
}

func (f *File) Head(ctx context.Context, projectID string) (_ Head, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.acquireLedgerLock(ctx, false)
	if err != nil {
		return Head{}, err
	}
	defer finishUnlock(unlock, &err)
	head, err := f.readHead()
	if err != nil {
		return Head{}, err
	}
	if isZeroHead(head) {
		return Head{}, nil
	}
	if err := validateFileHead(head); err != nil {
		return Head{}, err
	}
	if head.ProjectID != projectID {
		return Head{}, fmt.Errorf("%w: HEAD belongs to project %q, requested %q", ErrProjectConflict, head.ProjectID, projectID)
	}
	transaction, err := f.readTransaction(filepath.Join(f.transactions, head.TransactionID))
	if err != nil {
		return Head{}, fmt.Errorf("verify HEAD transaction: %w", err)
	}
	if transaction.Digest != head.Digest || transaction.Digest != digestTransaction(transaction) || transaction.Revision != head.Revision || transaction.ResumeDigest != head.ResumeDigest {
		return Head{}, errors.New("HEAD does not match its canonical transaction")
	}
	return Head{Revision: head.Revision, Digest: head.Digest, ResumeDigest: head.ResumeDigest}, nil
}

func (f *File) FindByIdempotency(ctx context.Context, projectID, idempotencyKey string) (_ Transaction, _ bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.acquireLedgerLock(ctx, false)
	if err != nil {
		return Transaction{}, false, err
	}
	defer finishUnlock(unlock, &err)
	transactions, _, err := f.readAll(ctx, projectID)
	if err != nil {
		return Transaction{}, false, err
	}
	for _, transaction := range transactions {
		if transaction.IdempotencyKey == idempotencyKey {
			return cloneTransaction(transaction), true, nil
		}
	}
	return Transaction{}, false, nil
}

func (f *File) Commit(ctx context.Context, draft Draft, expectedRevision uint64) (_ Transaction, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validateDraft(draft); err != nil {
		return Transaction{}, err
	}
	if !ledgerIDPattern.MatchString(draft.TransactionID) {
		return Transaction{}, errors.New("transaction id contains unsafe path characters")
	}
	unlock, err := f.acquireLedgerLock(ctx, true)
	if err != nil {
		return Transaction{}, err
	}
	defer finishUnlock(unlock, &err)
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	transactions, head, err := f.readAll(ctx, draft.ProjectID)
	if err != nil {
		return Transaction{}, err
	}
	for _, transaction := range transactions {
		if transaction.IdempotencyKey == draft.IdempotencyKey {
			if transaction.CommandDigest != draft.CommandDigest {
				return Transaction{}, ErrIdempotencyConflict
			}
			return cloneTransaction(transaction), nil
		}
		if transaction.TransactionID == draft.TransactionID {
			return Transaction{}, ErrTransactionIDConflict
		}
		if transaction.CommandID == draft.CommandID {
			return Transaction{}, ErrCommandIDConflict
		}
	}
	if head.Revision != expectedRevision {
		return Transaction{}, ErrRevisionConflict
	}
	transaction := Transaction{
		Draft:          cloneDraft(draft),
		Revision:       head.Revision + 1,
		PreviousDigest: head.Digest,
		CommittedAt:    time.Now().UTC(),
	}
	transaction.Digest = digestTransaction(transaction)
	marker := pendingCommit{
		Schema:         pendingCommitSchema,
		ProjectID:      transaction.ProjectID,
		TransactionID:  transaction.TransactionID,
		Revision:       transaction.Revision,
		PreviousDigest: transaction.PreviousDigest,
		Digest:         transaction.Digest,
	}
	if err := f.writePendingCommit(marker); err != nil {
		return Transaction{}, err
	}
	if err := f.writeTransaction(transaction); err != nil {
		return Transaction{}, err
	}
	if err := f.writeHead(fileHead{
		Schema:        ledgerHeadSchema,
		ProjectID:     draft.ProjectID,
		TransactionID: transaction.TransactionID,
		Revision:      transaction.Revision,
		Digest:        transaction.Digest,
		ResumeDigest:  transaction.ResumeDigest,
	}); err != nil {
		return Transaction{}, err
	}
	if err := f.clearPendingCommit(); err != nil {
		return Transaction{}, err
	}
	return cloneTransaction(transaction), nil
}

func (f *File) Transactions(ctx context.Context, projectID string) (_ []Transaction, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.acquireLedgerLock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer finishUnlock(unlock, &err)
	transactions, _, err := f.readAll(ctx, projectID)
	return transactions, err
}

func (f *File) LoadGenesis(ctx context.Context, ref GenesisRef, migrationID string) (_ Transaction, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validateGenesisRef(ref, migrationID); err != nil {
		return Transaction{}, err
	}
	unlock, err := f.acquireLedgerLock(ctx, false)
	if err != nil {
		return Transaction{}, err
	}
	defer finishUnlock(unlock, &err)

	journalPath := filepath.Join(f.runtime, "migration.rollback.json")
	journal, journalFound, err := readRollbackJournal(journalPath)
	if err != nil {
		return Transaction{}, err
	}
	if journalFound && (journal.Schema != "summer.migration-rollback/v1" || journal.MigrationID != migrationID || journal.Genesis != ref) {
		return Transaction{}, errors.New("rollback journal belongs to another genesis migration")
	}
	if !journalFound {
		transactions, head, readErr := f.readAll(ctx, ref.ProjectID)
		if readErr != nil {
			return Transaction{}, readErr
		}
		if len(transactions) != 1 || transactions[0].Revision != 1 || transactions[0].TransactionID != ref.TransactionID || transactions[0].Digest != ref.Digest || head.Revision != 1 || head.TransactionID != ref.TransactionID || head.Digest != ref.Digest {
			return Transaction{}, errors.New("rollback is allowed only for the unchanged migration genesis")
		}
		return cloneTransaction(transactions[0]), nil
	}

	live := filepath.Join(f.transactions, ref.TransactionID)
	archived := filepath.Join(filepath.Dir(f.root), "archive", "migrations", migrationID, "rollback", "v2", "transactions", ref.TransactionID)
	liveInfo, liveErr := os.Lstat(live)
	archivedInfo, archivedErr := os.Lstat(archived)
	var path string
	switch {
	case liveErr == nil && errors.Is(archivedErr, os.ErrNotExist):
		if !liveInfo.IsDir() || liveInfo.Mode()&os.ModeSymlink != 0 {
			return Transaction{}, errors.New("live migration genesis is unsafe")
		}
		path = live
	case errors.Is(liveErr, os.ErrNotExist) && archivedErr == nil:
		if !archivedInfo.IsDir() || archivedInfo.Mode()&os.ModeSymlink != 0 {
			return Transaction{}, errors.New("quarantined migration genesis is unsafe")
		}
		path = archived
	case liveErr == nil && archivedErr == nil:
		return Transaction{}, errors.New("migration genesis exists in both live and quarantine paths")
	default:
		return Transaction{}, errors.New("migration genesis is missing from live and quarantine paths")
	}
	transaction, err := f.readTransaction(path)
	if err != nil || transaction.ProjectID != ref.ProjectID || transaction.TransactionID != ref.TransactionID || transaction.Revision != 1 || transaction.Digest != ref.Digest {
		return Transaction{}, errors.New("migration genesis does not match the rollback journal")
	}
	return cloneTransaction(transaction), nil
}

func (f *File) QuarantineGenesis(ctx context.Context, ref GenesisRef, migrationID string) (err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validateGenesisRef(ref, migrationID); err != nil {
		return err
	}
	unlock, err := f.acquireLedgerLock(ctx, true)
	if err != nil {
		return err
	}
	defer finishUnlock(unlock, &err)
	journalPath := filepath.Join(f.runtime, "migration.rollback.json")
	journal, found, err := readRollbackJournal(journalPath)
	if err != nil {
		return err
	}
	if found {
		if journal.Schema != "summer.migration-rollback/v1" || journal.MigrationID != migrationID || journal.Genesis != ref {
			return errors.New("rollback journal belongs to another genesis migration")
		}
	} else {
		transactions, head, readErr := f.readAll(ctx, ref.ProjectID)
		if readErr != nil {
			return readErr
		}
		if len(transactions) != 1 || transactions[0].Revision != 1 || transactions[0].TransactionID != ref.TransactionID || transactions[0].Digest != ref.Digest || head.Revision != 1 || head.TransactionID != ref.TransactionID || head.Digest != ref.Digest {
			return errors.New("rollback is allowed only for the unchanged migration genesis")
		}
		if _, pending, pendingErr := f.readPendingCommit(); pendingErr != nil {
			return pendingErr
		} else if pending {
			return errors.New("rollback is not allowed while a commit recovery marker exists")
		}
		journal = rollbackJournal{Schema: "summer.migration-rollback/v1", MigrationID: migrationID, Genesis: ref, Stage: "prepared"}
	}

	archive := filepath.Join(filepath.Dir(f.root), "archive", "migrations", migrationID, "rollback", "v2")
	if err := makeRegularDirectories(filepath.Join(archive, "transactions"), 0o700); err != nil {
		return err
	}
	tombstonePath := filepath.Join(filepath.Dir(archive), "started.json")
	if err := ensureRollbackTombstone(tombstonePath, migrationID, ref); err != nil {
		return err
	}
	if !found {
		if err := writeAtomicJSONFile(journalPath, journal, 0o600); err != nil {
			return err
		}
	}
	transactionSource := filepath.Join(f.transactions, ref.TransactionID)
	transactionTarget := filepath.Join(archive, "transactions", ref.TransactionID)
	if journal.Stage == "prepared" {
		if err := moveTransactionForRollback(f, transactionSource, transactionTarget, ref); err != nil {
			return err
		}
		journal.Stage = "transaction_quarantined"
		if err := writeAtomicJSONFile(journalPath, journal, 0o600); err != nil {
			return err
		}
	} else if err := moveTransactionForRollback(f, transactionSource, transactionTarget, ref); err != nil {
		return err
	}
	if journal.Stage == "transaction_quarantined" {
		headTarget := filepath.Join(archive, "HEAD")
		if err := moveHeadForRollback(f, f.headPath, headTarget, ref); err != nil {
			return err
		}
		journal.Stage = "ledger_quarantined"
		if err := writeAtomicJSONFile(journalPath, journal, 0o600); err != nil {
			return err
		}
	} else if journal.Stage == "ledger_quarantined" || journal.Stage == "complete" {
		if err := moveHeadForRollback(f, f.headPath, filepath.Join(archive, "HEAD"), ref); err != nil {
			return err
		}
	}
	if journal.Stage != "ledger_quarantined" && journal.Stage != "complete" {
		return fmt.Errorf("unsupported rollback journal stage %q", journal.Stage)
	}
	return nil
}

func (f *File) CompleteGenesisQuarantine(ctx context.Context, ref GenesisRef, migrationID string) (err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.acquireLedgerLock(ctx, true)
	if err != nil {
		return err
	}
	defer finishUnlock(unlock, &err)
	journalPath := filepath.Join(f.runtime, "migration.rollback.json")
	journal, found, err := readRollbackJournal(journalPath)
	if err != nil {
		return err
	}
	if !found || journal.MigrationID != migrationID || journal.Genesis != ref || (journal.Stage != "ledger_quarantined" && journal.Stage != "complete") {
		return errors.New("rollback journal is not ready to complete")
	}
	archive := filepath.Join(filepath.Dir(f.root), "archive", "migrations", migrationID, "rollback", "v2")
	if err := moveTransactionForRollback(f, filepath.Join(f.transactions, ref.TransactionID), filepath.Join(archive, "transactions", ref.TransactionID), ref); err != nil {
		return err
	}
	if err := moveHeadForRollback(f, f.headPath, filepath.Join(archive, "HEAD"), ref); err != nil {
		return err
	}
	journal.Stage = "complete"
	return writeAtomicJSONFile(journalPath, journal, 0o600)
}

func validateGenesisRef(ref GenesisRef, migrationID string) error {
	if strings.TrimSpace(ref.ProjectID) == "" || !ledgerIDPattern.MatchString(ref.TransactionID) || strings.TrimSpace(ref.Digest) == "" || !ledgerIDPattern.MatchString(migrationID) {
		return errors.New("rollback genesis reference is invalid")
	}
	return nil
}

func readRollbackJournal(path string) (rollbackJournal, bool, error) {
	raw, err := readRegularFile(path, maxHeadBytes)
	if errors.Is(err, os.ErrNotExist) {
		return rollbackJournal{}, false, nil
	}
	if err != nil {
		return rollbackJournal{}, false, err
	}
	var journal rollbackJournal
	if err := decodeStrictJSON(raw, &journal); err != nil {
		return rollbackJournal{}, false, err
	}
	return journal, true, nil
}

func ensureRollbackTombstone(path, migrationID string, ref GenesisRef) error {
	raw, err := readRegularFile(path, maxHeadBytes)
	if errors.Is(err, os.ErrNotExist) {
		return writeAtomicJSONFile(path, rollbackTombstone{
			Schema: rollbackTombstoneSchema, MigrationID: migrationID, Genesis: ref,
		}, 0o600)
	}
	if err != nil {
		return err
	}
	var tombstone rollbackTombstone
	if err := decodeStrictJSON(raw, &tombstone); err != nil {
		return err
	}
	if tombstone.Schema != rollbackTombstoneSchema || tombstone.MigrationID != migrationID || tombstone.Genesis != ref {
		return errors.New("rollback tombstone belongs to another genesis migration")
	}
	return nil
}

func moveTransactionForRollback(f *File, source, target string, ref GenesisRef) error {
	sourceInfo, sourceErr := os.Lstat(source)
	targetInfo, targetErr := os.Lstat(target)
	switch {
	case sourceErr == nil && targetErr == nil:
		return errors.New("rollback transaction exists in both live and quarantine paths")
	case sourceErr == nil:
		if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 || !errors.Is(targetErr, os.ErrNotExist) {
			return errors.New("rollback transaction source or target is unsafe")
		}
		transaction, err := f.readTransaction(source)
		if err != nil || transaction.Revision != 1 || transaction.ProjectID != ref.ProjectID || transaction.TransactionID != ref.TransactionID || transaction.Digest != ref.Digest {
			return errors.New("rollback transaction does not match the migration genesis")
		}
		if err := os.Rename(source, target); err != nil {
			return fmt.Errorf("quarantine migration transaction: %w", err)
		}
		if err := fsyncDirectory(filepath.Dir(source)); err != nil {
			return err
		}
		return fsyncDirectory(filepath.Dir(target))
	case errors.Is(sourceErr, os.ErrNotExist) && targetErr == nil:
		if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("quarantined transaction is unsafe")
		}
		transaction, err := f.readTransaction(target)
		if err != nil || transaction.Revision != 1 || transaction.ProjectID != ref.ProjectID || transaction.TransactionID != ref.TransactionID || transaction.Digest != ref.Digest {
			return errors.New("quarantined transaction does not match the migration genesis")
		}
		return nil
	default:
		return errors.New("migration transaction is missing from live and quarantine paths")
	}
}

func moveHeadForRollback(f *File, source, target string, ref GenesisRef) error {
	sourceInfo, sourceErr := os.Lstat(source)
	targetInfo, targetErr := os.Lstat(target)
	validate := func(path string) error {
		raw, err := readRegularFile(path, maxHeadBytes)
		if err != nil {
			return err
		}
		var head fileHead
		if err := decodeStrictJSON(raw, &head); err != nil {
			return err
		}
		if head.ProjectID != ref.ProjectID || head.TransactionID != ref.TransactionID || head.Revision != 1 || head.Digest != ref.Digest {
			return errors.New("HEAD does not match the migration genesis")
		}
		return nil
	}
	switch {
	case sourceErr == nil && targetErr == nil:
		return errors.New("rollback HEAD exists in both live and quarantine paths")
	case sourceErr == nil:
		if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode()&os.ModeSymlink != 0 || !errors.Is(targetErr, os.ErrNotExist) {
			return errors.New("rollback HEAD source or target is unsafe")
		}
		if err := validate(source); err != nil {
			return err
		}
		if err := os.Rename(source, target); err != nil {
			return fmt.Errorf("quarantine migration HEAD: %w", err)
		}
		if err := fsyncDirectory(filepath.Dir(source)); err != nil {
			return err
		}
		return fsyncDirectory(filepath.Dir(target))
	case errors.Is(sourceErr, os.ErrNotExist) && targetErr == nil:
		if !targetInfo.Mode().IsRegular() || targetInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("quarantined HEAD is unsafe")
		}
		return validate(target)
	default:
		return errors.New("migration HEAD is missing from live and quarantine paths")
	}
}

func makeRegularDirectories(path string, mode os.FileMode) error {
	missing := make([]string, 0)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("rollback archive path is not a regular directory: %s", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := os.Mkdir(missing[index], mode); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(missing[index])
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if err != nil {
				return err
			}
			return fmt.Errorf("rollback archive path is not a regular directory: %s", missing[index])
		}
		if err := fsyncDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return nil
}

func finishUnlock(unlock func() error, err *error) {
	if unlockErr := unlock(); *err == nil && unlockErr != nil {
		*err = unlockErr
	}
}

func (f *File) acquireLedgerLock(ctx context.Context, recordOwner bool) (func() error, error) {
	if !recordOwner {
		return acquireProcessFileLock(ctx, f.lockPath, nil)
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("create ledger lock token: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("read hostname for ledger lock: %w", err)
	}
	owner := writeLockOwner{
		PID:       os.Getpid(),
		Token:     hex.EncodeToString(tokenBytes),
		Hostname:  hostname,
		CreatedAt: time.Now().UTC(),
	}
	return acquireProcessFileLock(ctx, f.lockPath, &owner)
}

func (f *File) readAll(ctx context.Context, projectID string) ([]Transaction, fileHead, error) {
	if err := ctx.Err(); err != nil {
		return nil, fileHead{}, err
	}
	directory, err := os.Open(f.transactions)
	if err != nil {
		return nil, fileHead{}, fmt.Errorf("open transaction directory: %w", err)
	}
	entries, readErr := directory.ReadDir(maxTransactions + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, fileHead{}, fmt.Errorf("read transaction directory: %w", readErr)
	}
	if closeErr != nil {
		return nil, fileHead{}, fmt.Errorf("close transaction directory: %w", closeErr)
	}
	if len(entries) > maxTransactions {
		return nil, fileHead{}, fmt.Errorf("transaction directory exceeds %d-entry limit", maxTransactions)
	}
	transactions := make([]Transaction, 0, len(entries))
	var ledgerBytes int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fileHead{}, err
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, fileHead{}, fmt.Errorf("unexpected non-directory transaction entry %q", entry.Name())
		}
		transaction, err := f.readTransaction(filepath.Join(f.transactions, entry.Name()))
		if err != nil {
			return nil, fileHead{}, err
		}
		if transaction.ProjectID != projectID {
			return nil, fileHead{}, fmt.Errorf("%w: ledger contains project %q, requested %q", ErrProjectConflict, transaction.ProjectID, projectID)
		}
		ledgerBytes += transactionFootprint(transaction)
		if ledgerBytes > maxLedgerBytes {
			return nil, fileHead{}, fmt.Errorf("ledger exceeds %d-byte in-memory limit", maxLedgerBytes)
		}
		transactions = append(transactions, transaction)
	}
	sort.Slice(transactions, func(i, j int) bool {
		if transactions[i].Revision == transactions[j].Revision {
			return transactions[i].TransactionID < transactions[j].TransactionID
		}
		return transactions[i].Revision < transactions[j].Revision
	})
	head, err := f.readHead()
	if err != nil {
		return nil, fileHead{}, err
	}
	if len(transactions) == 0 {
		if !isZeroHead(head) {
			return nil, fileHead{}, errors.New("HEAD exists without transactions")
		}
		if _, markerFound, markerErr := f.readPendingCommit(); markerErr != nil {
			if err := f.clearPendingCommit(); err != nil {
				return nil, fileHead{}, errors.Join(markerErr, fmt.Errorf("clear invalid transaction marker: %w", err))
			}
		} else if markerFound {
			if err := f.clearPendingCommit(); err != nil {
				return nil, fileHead{}, fmt.Errorf("clear stale transaction marker: %w", err)
			}
		}
		return transactions, head, nil
	}
	byDigest := make(map[string]Transaction, len(transactions))
	byID := make(map[string]Transaction, len(transactions))
	byCommandID := make(map[string]struct{}, len(transactions))
	byIdempotencyKey := make(map[string]struct{}, len(transactions))
	successorsByPrevious := make(map[string][]Transaction, len(transactions))
	for _, transaction := range transactions {
		if transaction.Digest != digestTransaction(transaction) {
			return nil, fileHead{}, fmt.Errorf("transaction %s digest mismatch", transaction.TransactionID)
		}
		if _, exists := byDigest[transaction.Digest]; exists {
			return nil, fileHead{}, fmt.Errorf("duplicate transaction digest %s", transaction.Digest)
		}
		if _, exists := byID[transaction.TransactionID]; exists {
			return nil, fileHead{}, fmt.Errorf("duplicate transaction id %s", transaction.TransactionID)
		}
		if _, exists := byCommandID[transaction.CommandID]; exists {
			return nil, fileHead{}, fmt.Errorf("duplicate command id %s", transaction.CommandID)
		}
		if _, exists := byIdempotencyKey[transaction.IdempotencyKey]; exists {
			return nil, fileHead{}, fmt.Errorf("duplicate idempotency key %s", transaction.IdempotencyKey)
		}
		byDigest[transaction.Digest] = transaction
		byID[transaction.TransactionID] = transaction
		byCommandID[transaction.CommandID] = struct{}{}
		byIdempotencyKey[transaction.IdempotencyKey] = struct{}{}
		successorsByPrevious[transaction.PreviousDigest] = append(successorsByPrevious[transaction.PreviousDigest], transaction)
	}

	chain := make([]Transaction, 0, len(transactions))
	committed := make(map[string]struct{}, len(transactions))
	if !isZeroHead(head) {
		if head.ProjectID != projectID {
			return nil, fileHead{}, fmt.Errorf("%w: HEAD belongs to project %q, requested %q", ErrProjectConflict, head.ProjectID, projectID)
		}
		tip, found := byID[head.TransactionID]
		if !found || tip.Revision != head.Revision || tip.Digest != head.Digest {
			return nil, fileHead{}, errors.New("HEAD does not reference a valid transaction")
		}
		for {
			chain = append(chain, tip)
			committed[tip.Digest] = struct{}{}
			if tip.Revision == 1 {
				if tip.PreviousDigest != "" {
					return nil, fileHead{}, fmt.Errorf("transaction %s genesis digest is not empty", tip.TransactionID)
				}
				break
			}
			previous, found := byDigest[tip.PreviousDigest]
			if !found || previous.Revision+1 != tip.Revision {
				return nil, fileHead{}, fmt.Errorf("transaction %s has no valid predecessor", tip.TransactionID)
			}
			tip = previous
		}
		for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
			chain[left], chain[right] = chain[right], chain[left]
		}
	}
	committedAtHead := len(chain)

	currentRevision := head.Revision
	currentDigest := head.Digest
	for {
		var successors []Transaction
		for _, transaction := range successorsByPrevious[currentDigest] {
			if _, exists := committed[transaction.Digest]; exists {
				continue
			}
			if transaction.Revision == currentRevision+1 && transaction.PreviousDigest == currentDigest {
				successors = append(successors, transaction)
			}
		}
		if len(successors) == 0 {
			break
		}
		if len(successors) > 1 {
			return nil, fileHead{}, fmt.Errorf("ledger fork after revision %d", currentRevision)
		}
		next := successors[0]
		chain = append(chain, next)
		committed[next.Digest] = struct{}{}
		currentRevision = next.Revision
		currentDigest = next.Digest
	}
	if len(committed) != len(transactions) {
		return nil, fileHead{}, errors.New("ledger contains transaction outside the HEAD chain")
	}
	marker, markerFound, err := f.readPendingCommit()
	adopted := chain[committedAtHead:]
	if err != nil {
		if len(adopted) > 0 {
			return nil, fileHead{}, fmt.Errorf("read recovery marker for orphan transaction: %w", err)
		}
		if clearErr := f.clearPendingCommit(); clearErr != nil {
			return nil, fileHead{}, errors.Join(err, fmt.Errorf("clear invalid transaction marker: %w", clearErr))
		}
		marker = pendingCommit{}
		markerFound = false
	}
	if len(adopted) > 0 {
		if !markerFound || len(adopted) != 1 || !marker.matches(adopted[0]) {
			return nil, fileHead{}, errors.New("orphan transaction is not authorized by the local recovery marker")
		}
	}
	last := chain[len(chain)-1]
	if head.Revision != last.Revision || head.Digest != last.Digest || head.TransactionID != last.TransactionID || head.ResumeDigest != last.ResumeDigest {
		if err := fsyncDirectory(f.transactions); err != nil {
			return nil, fileHead{}, fmt.Errorf("sync orphan transaction directory before adoption: %w", err)
		}
		head = fileHead{
			Schema:        ledgerHeadSchema,
			ProjectID:     projectID,
			TransactionID: last.TransactionID,
			Revision:      last.Revision,
			Digest:        last.Digest,
			ResumeDigest:  last.ResumeDigest,
		}
		if err := f.writeHead(head); err != nil {
			return nil, fileHead{}, fmt.Errorf("adopt orphan transaction: %w", err)
		}
		if err := f.clearPendingCommit(); err != nil {
			return nil, fileHead{}, fmt.Errorf("clear adopted transaction marker: %w", err)
		}
	} else if markerFound {
		if transaction, exists := byID[marker.TransactionID]; exists {
			if !marker.matches(transaction) || head.TransactionID != transaction.TransactionID || head.Digest != transaction.Digest {
				return nil, fileHead{}, errors.New("pending commit marker does not match canonical HEAD")
			}
		}
		if err := f.clearPendingCommit(); err != nil {
			return nil, fileHead{}, fmt.Errorf("clear stale transaction marker: %w", err)
		}
	}
	return chain, head, nil
}

func (pending pendingCommit) matches(transaction Transaction) bool {
	return pending.ProjectID == transaction.ProjectID &&
		pending.TransactionID == transaction.TransactionID &&
		pending.Revision == transaction.Revision &&
		pending.PreviousDigest == transaction.PreviousDigest &&
		pending.Digest == transaction.Digest
}

func isZeroHead(head fileHead) bool {
	return head.Schema == "" && head.ProjectID == "" && head.TransactionID == "" && head.Revision == 0 && head.Digest == "" && head.ResumeDigest == ""
}

func validateFileHead(head fileHead) error {
	if head.Schema != ledgerHeadSchema || strings.TrimSpace(head.ProjectID) == "" ||
		!ledgerIDPattern.MatchString(head.TransactionID) || head.Revision == 0 || strings.TrimSpace(head.Digest) == "" {
		return errors.New("HEAD is incomplete")
	}
	if head.ResumeDigest != "" {
		decoded, err := hex.DecodeString(head.ResumeDigest)
		if err != nil || len(decoded) != sha256.Size {
			return errors.New("HEAD resume digest is invalid")
		}
	}
	return nil
}

func (f *File) readHead() (fileHead, error) {
	raw, err := readRegularFile(f.headPath, maxHeadBytes)
	if errors.Is(err, os.ErrNotExist) {
		return fileHead{}, nil
	}
	if err != nil {
		return fileHead{}, fmt.Errorf("read HEAD: %w", err)
	}
	var head fileHead
	if err := decodeStrictJSON(raw, &head); err != nil {
		return fileHead{}, fmt.Errorf("decode HEAD: %w", err)
	}
	if head.Schema != ledgerHeadSchema {
		return fileHead{}, fmt.Errorf("unsupported HEAD schema %q", head.Schema)
	}
	return head, nil
}

func (f *File) readPendingCommit() (pendingCommit, bool, error) {
	raw, err := readRegularFile(f.pendingPath, maxHeadBytes)
	if errors.Is(err, os.ErrNotExist) {
		return pendingCommit{}, false, nil
	}
	if err != nil {
		return pendingCommit{}, false, fmt.Errorf("read pending commit marker: %w", err)
	}
	var pending pendingCommit
	if err := decodeStrictJSON(raw, &pending); err != nil {
		return pendingCommit{}, false, fmt.Errorf("decode pending commit marker: %w", err)
	}
	if pending.Schema != pendingCommitSchema {
		return pendingCommit{}, false, fmt.Errorf("unsupported pending commit schema %q", pending.Schema)
	}
	if strings.TrimSpace(pending.ProjectID) == "" || !ledgerIDPattern.MatchString(pending.TransactionID) || pending.Revision == 0 || pending.Digest == "" {
		return pendingCommit{}, false, errors.New("pending commit marker is incomplete")
	}
	return pending, true, nil
}

func (f *File) writePendingCommit(pending pendingCommit) error {
	if err := writeAtomicJSONFile(f.pendingPath, pending, 0o600); err != nil {
		return fmt.Errorf("write pending commit marker: %w", err)
	}
	return nil
}

func (f *File) clearPendingCommit() error {
	if err := os.Remove(f.pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pending commit marker: %w", err)
	}
	return fsyncDirectory(f.runtime)
}

func (f *File) readTransaction(directory string) (Transaction, error) {
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		return Transaction{}, fmt.Errorf("inspect transaction directory: %w", err)
	}
	if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
		return Transaction{}, errors.New("transaction path is not a regular directory")
	}
	directoryID := filepath.Base(directory)
	if !ledgerIDPattern.MatchString(directoryID) {
		return Transaction{}, errors.New("transaction directory name contains unsafe path characters")
	}
	raw, err := readRegularFile(filepath.Join(directory, "manifest.json"), maxManifestBytes)
	if err != nil {
		return Transaction{}, fmt.Errorf("read transaction manifest: %w", err)
	}
	var manifest fileManifest
	if err := decodeStrictJSON(raw, &manifest); err != nil {
		return Transaction{}, fmt.Errorf("decode transaction manifest: %w", err)
	}
	if manifest.Schema != transactionSchema {
		return Transaction{}, fmt.Errorf("unsupported transaction schema %q", manifest.Schema)
	}
	if manifest.TransactionID != directoryID {
		return Transaction{}, errors.New("transaction manifest id does not match its directory")
	}
	if len(manifest.EventFiles) == 0 || len(manifest.EventFiles) > maxEventsPerTransaction {
		return Transaction{}, fmt.Errorf("transaction event count %d is outside allowed bounds", len(manifest.EventFiles))
	}
	events := make([]Event, len(manifest.EventFiles))
	totalBytes := len(manifest.Actor)
	if totalBytes > maxTransactionDataBytes {
		return Transaction{}, fmt.Errorf("transaction data exceeds %d-byte limit", maxTransactionDataBytes)
	}
	for index, name := range manifest.EventFiles {
		wantName := fmt.Sprintf("%04d.json", index+1)
		if name != wantName {
			return Transaction{}, fmt.Errorf("transaction event file %q, want %q", name, wantName)
		}
		eventRaw, err := readRegularFile(filepath.Join(directory, name), maxEventBytes)
		if err != nil {
			return Transaction{}, fmt.Errorf("read transaction event: %w", err)
		}
		if err := decodeStrictJSON(eventRaw, &events[index]); err != nil {
			return Transaction{}, fmt.Errorf("decode transaction event: %w", err)
		}
		totalBytes += len(events[index].EventID) + len(events[index].Kind) + len(events[index].EntityID) + len(events[index].Data)
		if totalBytes > maxTransactionDataBytes {
			return Transaction{}, fmt.Errorf("transaction data exceeds %d-byte limit", maxTransactionDataBytes)
		}
	}
	transaction := Transaction{
		Draft: Draft{
			TransactionID:  manifest.TransactionID,
			ProjectID:      manifest.ProjectID,
			CommandID:      manifest.CommandID,
			CommandDigest:  manifest.CommandDigest,
			ResumeDigest:   manifest.ResumeDigest,
			IdempotencyKey: manifest.IdempotencyKey,
			CorrelationID:  manifest.CorrelationID,
			CausationID:    manifest.CausationID,
			IssuedAt:       manifest.IssuedAt,
			Actor:          manifest.Actor,
			Events:         events,
		},
		Revision:       manifest.Revision,
		PreviousDigest: manifest.PreviousDigest,
		Digest:         manifest.Digest,
		CommittedAt:    manifest.CommittedAt,
	}
	if err := validateDraft(transaction.Draft); err != nil {
		return Transaction{}, fmt.Errorf("validate transaction manifest: %w", err)
	}
	if transaction.Revision == 0 || transaction.Digest == "" || transaction.CommittedAt.IsZero() {
		return Transaction{}, errors.New("transaction manifest is missing revision, digest, or committed_at")
	}
	return transaction, nil
}

func decodeStrictJSON(raw []byte, destination any) error {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func readRegularFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds %d-byte limit: %s", maxBytes, path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d-byte limit: %s", maxBytes, path)
	}
	return raw, nil
}

func (f *File) writeTransaction(transaction Transaction) error {
	temporary, err := os.MkdirTemp(f.transactions, ".tx-")
	if err != nil {
		return fmt.Errorf("create transaction staging directory: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(temporary)
		}
	}()

	eventFiles := make([]string, len(transaction.Events))
	for index, event := range transaction.Events {
		name := fmt.Sprintf("%04d.json", index+1)
		eventFiles[index] = name
		if err := writeJSONFile(filepath.Join(temporary, name), event); err != nil {
			return err
		}
	}
	manifest := fileManifest{
		Schema:         transactionSchema,
		TransactionID:  transaction.TransactionID,
		ProjectID:      transaction.ProjectID,
		CommandID:      transaction.CommandID,
		CommandDigest:  transaction.CommandDigest,
		ResumeDigest:   transaction.ResumeDigest,
		IdempotencyKey: transaction.IdempotencyKey,
		CorrelationID:  transaction.CorrelationID,
		CausationID:    transaction.CausationID,
		IssuedAt:       transaction.IssuedAt,
		Actor:          transaction.Actor,
		Revision:       transaction.Revision,
		PreviousDigest: transaction.PreviousDigest,
		Digest:         transaction.Digest,
		CommittedAt:    transaction.CommittedAt,
		EventFiles:     eventFiles,
	}
	if err := writeJSONFile(filepath.Join(temporary, "manifest.json"), manifest); err != nil {
		return err
	}
	if err := fsyncDirectory(temporary); err != nil {
		return err
	}
	final := filepath.Join(f.transactions, transaction.TransactionID)
	if err := os.Rename(temporary, final); err != nil {
		return fmt.Errorf("commit transaction directory: %w", err)
	}
	committed = true
	return fsyncDirectory(f.transactions)
}

func (f *File) writeHead(head fileHead) error {
	raw, err := json.MarshalIndent(head, "", "  ")
	if err != nil {
		return fmt.Errorf("encode HEAD: %w", err)
	}
	temporary, err := os.CreateTemp(f.root, ".HEAD-")
	if err != nil {
		return fmt.Errorf("create HEAD staging file: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		temporary.Close()
		return fmt.Errorf("write HEAD: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync HEAD: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close HEAD: %w", err)
	}
	if err := os.Rename(name, f.headPath); err != nil {
		return fmt.Errorf("replace HEAD: %w", err)
	}
	return fsyncDirectory(f.root)
}

func writeJSONFile(path string, value any) error {
	return writeJSONFileWithMode(path, value, 0o644)
}

func writeJSONFileWithMode(path string, value any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if _, err := file.Write(append(raw, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync %s: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeAtomicJSONFile(path string, value any, mode os.FileMode) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+"-")
	if err != nil {
		return fmt.Errorf("create %s staging file: %w", filepath.Base(path), err)
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return fmt.Errorf("set %s mode: %w", filepath.Base(path), err)
	}
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write %s staging file: %w", filepath.Base(path), err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync %s staging file: %w", filepath.Base(path), err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s staging file: %w", filepath.Base(path), err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	committed = true
	return fsyncDirectory(directory)
}

func fsyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
