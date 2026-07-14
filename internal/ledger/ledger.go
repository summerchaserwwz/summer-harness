package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRevisionConflict      = errors.New("ledger revision conflict")
	ErrIdempotencyConflict   = errors.New("ledger idempotency conflict")
	ErrTransactionIDConflict = errors.New("ledger transaction id conflict")
	ErrCommandIDConflict     = errors.New("ledger command id conflict")
	ErrProjectConflict       = errors.New("ledger project conflict")
)

const (
	maxEventsPerTransaction = 256
	maxEventDataBytes       = 1 << 20
	maxTransactionDataBytes = 16 << 20
	maxLedgerFieldBytes     = 512
	maxActorBytes           = 64 << 10
)

type Event struct {
	EventID  string          `json:"event_id"`
	Kind     string          `json:"kind"`
	EntityID string          `json:"entity_id"`
	Data     json.RawMessage `json:"data"`
}

type Draft struct {
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
	Events         []Event         `json:"events"`
}

type Transaction struct {
	Draft
	Revision       uint64    `json:"revision"`
	PreviousDigest string    `json:"previous_digest"`
	Digest         string    `json:"digest"`
	CommittedAt    time.Time `json:"committed_at"`
}

type Head struct {
	Revision     uint64 `json:"revision"`
	Digest       string `json:"digest"`
	ResumeDigest string `json:"resume_digest,omitempty"`
}

type GenesisRef struct {
	ProjectID     string `json:"project_id"`
	TransactionID string `json:"transaction_id"`
	Digest        string `json:"digest"`
}

type GenesisQuarantiner interface {
	LoadGenesis(ctx context.Context, ref GenesisRef, migrationID string) (Transaction, error)
	QuarantineGenesis(ctx context.Context, ref GenesisRef, migrationID string) error
	CompleteGenesisQuarantine(ctx context.Context, ref GenesisRef, migrationID string) error
}

type Store interface {
	Project(ctx context.Context) (projectID string, found bool, err error)
	Head(ctx context.Context, projectID string) (Head, error)
	FindByIdempotency(ctx context.Context, projectID, idempotencyKey string) (Transaction, bool, error)
	Commit(ctx context.Context, draft Draft, expectedRevision uint64) (Transaction, error)
	Transactions(ctx context.Context, projectID string) ([]Transaction, error)
}

// ValidateDraft exposes the canonical transaction limits to command builders
// that must fail before a durable write, such as legacy migration dry-runs.
func ValidateDraft(draft Draft) error {
	return validateDraft(draft)
}

func validateDraft(draft Draft) error {
	for label, value := range map[string]string{
		"transaction_id":  draft.TransactionID,
		"project_id":      draft.ProjectID,
		"command_id":      draft.CommandID,
		"command_digest":  draft.CommandDigest,
		"idempotency_key": draft.IdempotencyKey,
		"correlation_id":  draft.CorrelationID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		if len(value) > maxLedgerFieldBytes {
			return fmt.Errorf("%s exceeds %d-byte limit", label, maxLedgerFieldBytes)
		}
	}
	if draft.IssuedAt.IsZero() {
		return errors.New("issued_at is required")
	}
	if draft.ResumeDigest != "" {
		decoded, err := hex.DecodeString(draft.ResumeDigest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("resume_digest must be a %d-byte SHA-256 digest", sha256.Size)
		}
	}
	if len(draft.CausationID) > maxLedgerFieldBytes {
		return fmt.Errorf("causation_id exceeds %d-byte limit", maxLedgerFieldBytes)
	}
	if len(draft.Actor) == 0 || len(draft.Actor) > maxActorBytes || !json.Valid(draft.Actor) {
		return errors.New("actor must be valid JSON")
	}
	if len(draft.Events) == 0 || len(draft.Events) > maxEventsPerTransaction {
		return fmt.Errorf("transaction event count %d is outside allowed bounds", len(draft.Events))
	}
	totalBytes := len(draft.Actor)
	for index, event := range draft.Events {
		if strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.Kind) == "" || strings.TrimSpace(event.EntityID) == "" {
			return fmt.Errorf("event %d is missing id, kind, or entity id", index)
		}
		if len(event.EventID) > maxLedgerFieldBytes || len(event.Kind) > maxLedgerFieldBytes || len(event.EntityID) > maxLedgerFieldBytes {
			return fmt.Errorf("event %d id, kind, or entity id exceeds %d-byte limit", index, maxLedgerFieldBytes)
		}
		if len(event.Data) > maxEventDataBytes {
			return fmt.Errorf("event %d data exceeds %d-byte limit", index, maxEventDataBytes)
		}
		if !json.Valid(event.Data) {
			return fmt.Errorf("event %d data must be valid JSON", index)
		}
		totalBytes += len(event.EventID) + len(event.Kind) + len(event.EntityID) + len(event.Data)
		if totalBytes > maxTransactionDataBytes {
			return fmt.Errorf("transaction data exceeds %d-byte limit", maxTransactionDataBytes)
		}
	}
	return nil
}

func transactionFootprint(transaction Transaction) int64 {
	total := int64(len(transaction.Actor))
	for _, event := range transaction.Events {
		total += int64(len(event.EventID) + len(event.Kind) + len(event.EntityID) + len(event.Data))
	}
	return total
}

func digestTransaction(transaction Transaction) string {
	copy := transaction
	copy.Digest = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
