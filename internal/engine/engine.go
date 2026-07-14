package engine

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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

const CommandSchemaV2 = "summer.command/v2"

const (
	maxObjectiveTitleChars      = 200
	maxObjectiveTextChars       = 2000
	maxObjectiveAcceptanceItems = 32
	maxProfileChars             = 64
	maxCommandPayloadBytes      = 4 << 20
	maxEnvelopeIDBytes          = 512
)

var highConfidenceSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{20,}\b`),
	regexp.MustCompile(`\bsk_live_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
}

type ActorRole string

const (
	ActorUser        ActorRole = "user"
	ActorCoordinator ActorRole = "coordinator"
	ActorWorker      ActorRole = "worker"
	ActorReviewer    ActorRole = "reviewer"
	ActorSystem      ActorRole = "system"
)

type ActorRef struct {
	ActorID   string    `json:"actor_id"`
	SessionID string    `json:"session_id"`
	Runtime   string    `json:"runtime"`
	Model     string    `json:"model,omitempty"`
	Role      ActorRole `json:"role"`
}

type CommandKind string

const CommandStartObjective CommandKind = "StartObjective"

type CommandEnvelope struct {
	Schema           string          `json:"schema"`
	CommandID        string          `json:"command_id"`
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

type StartObjective struct {
	Title      string   `json:"title"`
	Goal       string   `json:"goal"`
	Acceptance []string `json:"acceptance"`
	Profile    string   `json:"profile"`
}

type ObjectiveStatus string

const ObjectiveActive ObjectiveStatus = "active"

type Objective struct {
	ObjectiveID string          `json:"objective_id"`
	ProjectID   string          `json:"project_id"`
	Title       string          `json:"title"`
	Goal        string          `json:"goal"`
	Acceptance  []string        `json:"acceptance"`
	Profile     string          `json:"profile"`
	Status      ObjectiveStatus `json:"status"`
	Revision    uint64          `json:"revision"`
}

type Rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Receipt struct {
	Accepted        bool       `json:"accepted"`
	TransactionID   string     `json:"transaction_id,omitempty"`
	NewRevision     uint64     `json:"new_revision,omitempty"`
	EntityID        string     `json:"entity_id,omitempty"`
	EmittedEventIDs []string   `json:"emitted_event_ids"`
	Rejection       *Rejection `json:"rejection,omitempty"`
}

type QueryKind string

const QueryObjective QueryKind = "Objective"

type Query struct {
	Kind      QueryKind `json:"kind"`
	ProjectID string    `json:"project_id"`
	EntityID  string    `json:"entity_id,omitempty"`
}

type View interface {
	QueryKind() QueryKind
	isView()
}

type ObjectiveView struct {
	Objective Objective `json:"objective"`
}

func (ObjectiveView) QueryKind() QueryKind { return QueryObjective }
func (ObjectiveView) isView()              {}

type Engine interface {
	Apply(ctx context.Context, command CommandEnvelope) (Receipt, error)
	Query(ctx context.Context, query Query) (View, error)
}

type kernel struct {
	store ledger.Store
}

func New(store ledger.Store) Engine {
	return &kernel{store: store}
}

func (k *kernel) Apply(ctx context.Context, command CommandEnvelope) (Receipt, error) {
	if rejection := validateEnvelope(command); rejection != nil {
		return Receipt{Accepted: false, Rejection: rejection}, nil
	}
	if commandContainsHighConfidenceSecret(command) {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "SENSITIVE_CONTENT",
			Message: "command contains a high-confidence secret pattern and was not written",
		}}, nil
	}
	digest, err := commandDigest(command)
	if err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "INVALID_COMMAND",
			Message: err.Error(),
		}}, nil
	}
	if receipt, found, err := k.findIdempotentReceipt(ctx, command, digest); err != nil {
		return Receipt{}, err
	} else if found {
		return receipt, nil
	}
	if command.Kind != CommandStartObjective {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "UNSUPPORTED_COMMAND",
			Message: fmt.Sprintf("unsupported command kind %q", command.Kind),
		}}, nil
	}
	if command.Actor.Role != ActorUser && command.Actor.Role != ActorCoordinator {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "FORBIDDEN",
			Message: "only a user or coordinator can start the root objective",
		}}, nil
	}

	var start StartObjective
	if err := json.Unmarshal(command.Payload, &start); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "INVALID_COMMAND",
			Message: fmt.Sprintf("decode StartObjective: %v", err),
		}}, nil
	}
	if err := validateStartObjective(start); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "INVALID_COMMAND",
			Message: err.Error(),
		}}, nil
	}
	secretInputs := append([]string{start.Title, start.Goal, start.Profile}, start.Acceptance...)
	if containsHighConfidenceSecret(secretInputs...) {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "SENSITIVE_CONTENT",
			Message: "objective contains a high-confidence secret pattern and was not written",
		}}, nil
	}
	transactions, err := k.store.Transactions(ctx, command.ProjectID)
	if err != nil {
		return Receipt{}, err
	}
	observedRevision := uint64(0)
	if len(transactions) > 0 {
		observedRevision = transactions[len(transactions)-1].Revision
	}
	if observedRevision != command.ExpectedRevision {
		if receipt, found, err := k.findIdempotentReceipt(ctx, command, digest); err != nil {
			return Receipt{}, err
		} else if found {
			return receipt, nil
		}
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "REVISION_CONFLICT",
			Message: "expected revision does not match the state used for validation",
		}}, nil
	}
	for _, transaction := range transactions {
		for _, event := range transaction.Events {
			if event.Kind == "ObjectiveStarted" {
				if receipt, found, err := k.findIdempotentReceipt(ctx, command, digest); err != nil {
					return Receipt{}, err
				} else if found {
					return receipt, nil
				}
				return Receipt{Accepted: false, Rejection: &Rejection{
					Code:    "OBJECTIVE_EXISTS",
					Message: "project already has a root objective",
				}}, nil
			}
		}
	}

	objectiveID, err := newID("obj")
	if err != nil {
		return Receipt{}, err
	}
	objective := Objective{
		ObjectiveID: objectiveID,
		ProjectID:   command.ProjectID,
		Title:       strings.TrimSpace(start.Title),
		Goal:        strings.TrimSpace(start.Goal),
		Acceptance:  cleanStrings(start.Acceptance),
		Profile:     strings.TrimSpace(start.Profile),
		Status:      ObjectiveActive,
		Revision:    1,
	}
	data, err := json.Marshal(objective)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode objective: %w", err)
	}
	actor, err := json.Marshal(command.Actor)
	if err != nil {
		return Receipt{}, fmt.Errorf("encode actor: %w", err)
	}
	eventID, err := newID("evt")
	if err != nil {
		return Receipt{}, err
	}
	transactionID, err := newID("tx")
	if err != nil {
		return Receipt{}, err
	}
	transaction, err := k.store.Commit(ctx, ledger.Draft{
		TransactionID:  transactionID,
		ProjectID:      command.ProjectID,
		CommandID:      command.CommandID,
		CommandDigest:  digest,
		IdempotencyKey: command.IdempotencyKey,
		CorrelationID:  command.CorrelationID,
		CausationID:    command.CausationID,
		IssuedAt:       command.IssuedAt,
		Actor:          actor,
		Events: []ledger.Event{{
			EventID:  eventID,
			Kind:     "ObjectiveStarted",
			EntityID: objectiveID,
			Data:     data,
		}},
	}, command.ExpectedRevision)
	if errors.Is(err, ledger.ErrIdempotencyConflict) {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "IDEMPOTENCY_CONFLICT",
			Message: "idempotency key was already used for a different command",
		}}, nil
	}
	if errors.Is(err, ledger.ErrRevisionConflict) {
		if receipt, found, findErr := k.findIdempotentReceipt(ctx, command, digest); findErr != nil {
			return Receipt{}, findErr
		} else if found {
			return receipt, nil
		}
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "REVISION_CONFLICT",
			Message: "expected revision does not match ledger head",
		}}, nil
	}
	if err != nil {
		return Receipt{}, err
	}
	if transaction.TransactionID != transactionID {
		return receiptFromTransaction(transaction), nil
	}
	return Receipt{
		Accepted:        true,
		TransactionID:   transaction.TransactionID,
		NewRevision:     transaction.Revision,
		EntityID:        objectiveID,
		EmittedEventIDs: []string{eventID},
	}, nil
}

func (k *kernel) findIdempotentReceipt(ctx context.Context, command CommandEnvelope, digest string) (Receipt, bool, error) {
	transaction, found, err := k.store.FindByIdempotency(ctx, command.ProjectID, command.IdempotencyKey)
	if err != nil || !found {
		return Receipt{}, false, err
	}
	if transaction.CommandDigest != digest {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "IDEMPOTENCY_CONFLICT",
			Message: "idempotency key was already used for a different command",
		}}, true, nil
	}
	return receiptFromTransaction(transaction), true, nil
}

func receiptFromTransaction(transaction ledger.Transaction) Receipt {
	receipt := Receipt{
		Accepted:      true,
		TransactionID: transaction.TransactionID,
		NewRevision:   transaction.Revision,
	}
	for _, event := range transaction.Events {
		receipt.EmittedEventIDs = append(receipt.EmittedEventIDs, event.EventID)
		if receipt.EntityID == "" {
			receipt.EntityID = event.EntityID
		}
	}
	return receipt
}

func (k *kernel) Query(ctx context.Context, query Query) (View, error) {
	if query.Kind != QueryObjective {
		return nil, fmt.Errorf("unsupported query kind %q", query.Kind)
	}
	transactions, err := k.store.Transactions(ctx, query.ProjectID)
	if err != nil {
		return nil, err
	}
	for _, transaction := range transactions {
		for _, event := range transaction.Events {
			if event.Kind != "ObjectiveStarted" || event.EntityID != query.EntityID {
				continue
			}
			var objective Objective
			if err := json.Unmarshal(event.Data, &objective); err != nil {
				return nil, fmt.Errorf("decode ObjectiveStarted event: %w", err)
			}
			return ObjectiveView{Objective: objective}, nil
		}
	}
	return nil, fmt.Errorf("objective %q not found", query.EntityID)
}

func validateEnvelope(command CommandEnvelope) *Rejection {
	if command.Schema != CommandSchemaV2 {
		return &Rejection{Code: "UNSUPPORTED_SCHEMA", Message: fmt.Sprintf("unsupported command schema %q", command.Schema)}
	}
	for label, value := range map[string]string{
		"command_id": command.CommandID, "idempotency_key": command.IdempotencyKey,
		"correlation_id": command.CorrelationID, "project_id": command.ProjectID,
		"actor_id": command.Actor.ActorID, "session_id": command.Actor.SessionID,
		"runtime": command.Actor.Runtime,
	} {
		if strings.TrimSpace(value) == "" {
			return &Rejection{Code: "INVALID_COMMAND", Message: fmt.Sprintf("%s is required", label)}
		}
		if len(value) > maxEnvelopeIDBytes {
			return &Rejection{Code: "INVALID_COMMAND", Message: fmt.Sprintf("%s exceeds %d bytes", label, maxEnvelopeIDBytes)}
		}
	}
	if len(command.CausationID) > maxEnvelopeIDBytes {
		return &Rejection{Code: "INVALID_COMMAND", Message: fmt.Sprintf("causation_id exceeds %d bytes", maxEnvelopeIDBytes)}
	}
	if len(command.Actor.Model) > maxEnvelopeIDBytes {
		return &Rejection{Code: "INVALID_ACTOR", Message: fmt.Sprintf("model exceeds %d bytes", maxEnvelopeIDBytes)}
	}
	switch command.Actor.Role {
	case ActorUser, ActorCoordinator, ActorWorker, ActorReviewer, ActorSystem:
	default:
		return &Rejection{Code: "INVALID_ACTOR", Message: fmt.Sprintf("unsupported actor role %q", command.Actor.Role)}
	}
	if command.IssuedAt.IsZero() {
		return &Rejection{Code: "INVALID_COMMAND", Message: "issued_at is required"}
	}
	if len(command.Payload) == 0 || len(command.Payload) > maxCommandPayloadBytes {
		return &Rejection{Code: "INVALID_COMMAND", Message: fmt.Sprintf("payload must be between 1 and %d bytes", maxCommandPayloadBytes)}
	}
	return nil
}

func commandDigest(command CommandEnvelope) (string, error) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(command.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "", fmt.Errorf("decode command payload for digest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", errors.New("decode command payload for digest: multiple JSON values")
		}
		return "", fmt.Errorf("decode command payload for digest: %w", err)
	}
	canonicalPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode command payload for digest: %w", err)
	}
	canonical := struct {
		Schema           string          `json:"schema"`
		CommandID        string          `json:"command_id"`
		IdempotencyKey   string          `json:"idempotency_key"`
		CorrelationID    string          `json:"correlation_id"`
		CausationID      string          `json:"causation_id,omitempty"`
		ProjectID        string          `json:"project_id"`
		ExpectedRevision uint64          `json:"expected_revision"`
		Actor            ActorRef        `json:"actor"`
		IssuedAt         time.Time       `json:"issued_at"`
		Kind             CommandKind     `json:"kind"`
		Payload          json.RawMessage `json:"payload"`
	}{
		Schema:           command.Schema,
		CommandID:        command.CommandID,
		IdempotencyKey:   command.IdempotencyKey,
		CorrelationID:    command.CorrelationID,
		CausationID:      command.CausationID,
		ProjectID:        command.ProjectID,
		ExpectedRevision: command.ExpectedRevision,
		Actor:            command.Actor,
		IssuedAt:         command.IssuedAt,
		Kind:             command.Kind,
		Payload:          canonicalPayload,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode command digest input: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func validateStartObjective(start StartObjective) error {
	if err := validateBoundedText(start.Title, "objective title", maxObjectiveTitleChars); err != nil {
		return err
	}
	if err := validateBoundedText(start.Goal, "objective goal", maxObjectiveTextChars); err != nil {
		return err
	}
	acceptance := cleanStrings(start.Acceptance)
	if len(acceptance) == 0 {
		return errors.New("objective acceptance is required")
	}
	if len(acceptance) > maxObjectiveAcceptanceItems {
		return fmt.Errorf("objective acceptance exceeds %d items", maxObjectiveAcceptanceItems)
	}
	for _, criterion := range acceptance {
		if err := validateBoundedText(criterion, "objective acceptance criterion", maxObjectiveTextChars); err != nil {
			return err
		}
	}
	if err := validateBoundedText(start.Profile, "objective profile", maxProfileChars); err != nil {
		return err
	}
	switch strings.TrimSpace(start.Profile) {
	case "standard", "research", "high-risk", "release":
	default:
		return fmt.Errorf("unsupported objective profile %q", start.Profile)
	}
	return nil
}

func validateBoundedText(value, label string, maxChars int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if utf8.RuneCountInString(value) > maxChars {
		return fmt.Errorf("%s exceeds %d characters", label, maxChars)
	}
	return nil
}

func containsHighConfidenceSecret(values ...string) bool {
	for _, value := range values {
		if strings.Contains(value, "-----BEGIN PRIVATE KEY-----") || strings.Contains(value, "-----BEGIN RSA PRIVATE KEY-----") || strings.Contains(value, "-----BEGIN OPENSSH PRIVATE KEY-----") {
			return true
		}
		for _, pattern := range highConfidenceSecretPatterns {
			if pattern.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func commandContainsHighConfidenceSecret(command CommandEnvelope) bool {
	return containsHighConfidenceSecret(
		command.CommandID,
		command.IdempotencyKey,
		command.CorrelationID,
		command.CausationID,
		command.ProjectID,
		command.Actor.ActorID,
		command.Actor.SessionID,
		command.Actor.Runtime,
		command.Actor.Model,
		string(command.Payload),
	)
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func newID(prefix string) (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("create %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(random), nil
}
