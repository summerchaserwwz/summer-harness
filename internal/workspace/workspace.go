package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

func Open(root string) (engine.Engine, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	module, err := continuity.NewFile(resolved)
	if err != nil {
		return nil, err
	}
	store := &lazyFileStore{root: filepath.Join(resolved, ".agent", "ledger")}
	return engine.New(store, engine.WithContinuity(module)), nil
}

type lazyFileStore struct {
	root  string
	once  sync.Once
	store *ledger.File
	err   error
}

func (s *lazyFileStore) Project(ctx context.Context) (string, bool, error) {
	found, err := s.hasFileLedger()
	if err != nil || !found {
		return "", false, err
	}
	store, err := s.get()
	if err != nil {
		return "", false, err
	}
	return store.Project(ctx)
}

func (s *lazyFileStore) hasFileLedger() (bool, error) {
	paths := []string{
		filepath.Join(s.root, "HEAD"),
		filepath.Join(s.root, "transactions"),
		filepath.Join(filepath.Dir(s.root), "runtime", "ledger.pending.json"),
	}
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (s *lazyFileStore) get() (*ledger.File, error) {
	s.once.Do(func() { s.store, s.err = ledger.NewFile(s.root) })
	return s.store, s.err
}

func (s *lazyFileStore) Head(ctx context.Context, projectID string) (ledger.Head, error) {
	found, err := s.hasFileLedger()
	if err != nil || !found {
		return ledger.Head{}, err
	}
	store, err := s.get()
	if err != nil {
		return ledger.Head{}, err
	}
	return store.Head(ctx, projectID)
}

func (s *lazyFileStore) FindByIdempotency(ctx context.Context, projectID, key string) (ledger.Transaction, bool, error) {
	found, err := s.hasFileLedger()
	if err != nil || !found {
		return ledger.Transaction{}, false, err
	}
	store, err := s.get()
	if err != nil {
		return ledger.Transaction{}, false, err
	}
	return store.FindByIdempotency(ctx, projectID, key)
}

func (s *lazyFileStore) Commit(ctx context.Context, draft ledger.Draft, revision uint64) (ledger.Transaction, error) {
	store, err := s.get()
	if err != nil {
		return ledger.Transaction{}, err
	}
	return store.Commit(ctx, draft, revision)
}

func (s *lazyFileStore) Transactions(ctx context.Context, projectID string) ([]ledger.Transaction, error) {
	found, err := s.hasFileLedger()
	if err != nil || !found {
		return []ledger.Transaction{}, err
	}
	store, err := s.get()
	if err != nil {
		return nil, err
	}
	return store.Transactions(ctx, projectID)
}
