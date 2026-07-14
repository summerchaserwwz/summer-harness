package ledger

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type Memory struct {
	mu           sync.RWMutex
	projectID    string
	transactions []Transaction
}

func NewMemory() *Memory {
	return &Memory{}
}

func (m *Memory) Head(ctx context.Context, projectID string) (Head, error) {
	if err := ctx.Err(); err != nil {
		return Head{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Head{}, err
	}
	if err := m.validateProject(projectID); err != nil {
		return Head{}, err
	}
	return headOf(m.transactions), nil
}

func (m *Memory) FindByIdempotency(ctx context.Context, projectID, idempotencyKey string) (Transaction, bool, error) {
	if err := ctx.Err(); err != nil {
		return Transaction{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Transaction{}, false, err
	}
	if err := m.validateProject(projectID); err != nil {
		return Transaction{}, false, err
	}
	for _, transaction := range m.transactions {
		if transaction.IdempotencyKey == idempotencyKey {
			return cloneTransaction(transaction), true, nil
		}
	}
	return Transaction{}, false, nil
}

func (m *Memory) Commit(ctx context.Context, draft Draft, expectedRevision uint64) (Transaction, error) {
	if err := validateDraft(draft); err != nil {
		return Transaction{}, err
	}
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Transaction{}, err
	}

	if err := m.validateProject(draft.ProjectID); err != nil {
		return Transaction{}, err
	}
	for _, transaction := range m.transactions {
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
	head := headOf(m.transactions)
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
	if m.projectID == "" {
		m.projectID = draft.ProjectID
	}
	m.transactions = append(m.transactions, transaction)
	return cloneTransaction(transaction), nil
}

func (m *Memory) Transactions(ctx context.Context, projectID string) ([]Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.validateProject(projectID); err != nil {
		return nil, err
	}
	result := make([]Transaction, len(m.transactions))
	for index, transaction := range m.transactions {
		result[index] = cloneTransaction(transaction)
	}
	return result, nil
}

func (m *Memory) validateProject(projectID string) error {
	if m.projectID != "" && m.projectID != projectID {
		return ErrProjectConflict
	}
	return nil
}

func headOf(transactions []Transaction) Head {
	if len(transactions) == 0 {
		return Head{}
	}
	last := transactions[len(transactions)-1]
	return Head{Revision: last.Revision, Digest: last.Digest}
}

func cloneDraft(draft Draft) Draft {
	copy := draft
	copy.Actor = append(json.RawMessage(nil), draft.Actor...)
	copy.Events = make([]Event, len(draft.Events))
	for index, event := range draft.Events {
		copy.Events[index] = event
		copy.Events[index].Data = append(json.RawMessage(nil), event.Data...)
	}
	return copy
}

func cloneTransaction(transaction Transaction) Transaction {
	copy := transaction
	copy.Draft = cloneDraft(transaction.Draft)
	return copy
}
