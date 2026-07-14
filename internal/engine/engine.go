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

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
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

const (
	CommandStartObjective          CommandKind = "StartObjective"
	CommandSaveObjective           CommandKind = "SaveObjective"
	CommandImportLegacyNative      CommandKind = "ImportLegacyNative"
	CommandRollbackLegacyMigration CommandKind = "RollbackLegacyMigration"
)

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
	Next       []string `json:"next,omitempty"`
	Profile    string   `json:"profile"`
}

type SaveObjective struct {
	ObjectiveID               string    `json:"objective_id"`
	ExpectedObjectiveRevision uint64    `json:"expected_objective_revision"`
	Done                      []string  `json:"done,omitempty"`
	ReplaceDone               bool      `json:"replace_done,omitempty"`
	Next                      *[]string `json:"next,omitempty"`
	Validation                []string  `json:"validation,omitempty"`
	ReplaceValidation         bool      `json:"replace_validation,omitempty"`
	Blockers                  *[]string `json:"blockers,omitempty"`
	MustRead                  *[]string `json:"must_read,omitempty"`
}

type ImportLegacyNative struct {
	MigrationID          string `json:"migration_id"`
	SourceDigest         string `json:"source_digest"`
	BackupManifestDigest string `json:"backup_manifest_digest"`
}

type RollbackLegacyMigration struct {
	MigrationID           string `json:"migration_id"`
	ExpectedTransactionID string `json:"expected_transaction_id"`
	ExpectedLedgerHead    string `json:"expected_ledger_head"`
}

type ObjectiveStatus string

const (
	ObjectiveActive    ObjectiveStatus = "active"
	ObjectiveBlocked   ObjectiveStatus = "blocked"
	ObjectiveReview    ObjectiveStatus = "review"
	ObjectiveCompleted ObjectiveStatus = "completed"
	ObjectiveCancelled ObjectiveStatus = "cancelled"
)

type Objective struct {
	ObjectiveID   string          `json:"objective_id"`
	ProjectID     string          `json:"project_id"`
	Title         string          `json:"title"`
	Goal          string          `json:"goal"`
	Acceptance    []string        `json:"acceptance"`
	Profile       string          `json:"profile"`
	Risk          string          `json:"risk,omitempty"`
	Status        ObjectiveStatus `json:"status"`
	Revision      uint64          `json:"revision"`
	Done          []string        `json:"done"`
	Next          []string        `json:"next"`
	Validation    []string        `json:"validation"`
	Blockers      []string        `json:"blockers"`
	MustRead      []string        `json:"must_read"`
	ResidualRisks []string        `json:"residual_risks,omitempty"`
}

type Rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Receipt struct {
	Accepted        bool               `json:"accepted"`
	TransactionID   string             `json:"transaction_id,omitempty"`
	NewRevision     uint64             `json:"new_revision,omitempty"`
	EntityID        string             `json:"entity_id,omitempty"`
	EntityRevision  uint64             `json:"entity_revision,omitempty"`
	EntityStatus    string             `json:"entity_status,omitempty"`
	EmittedEventIDs []string           `json:"emitted_event_ids"`
	Rejection       *Rejection         `json:"rejection,omitempty"`
	Projection      *ProjectionReceipt `json:"projection,omitempty"`
}

type ProjectionStatus string

const (
	ProjectionCurrent        ProjectionStatus = "current"
	ProjectionRepairRequired ProjectionStatus = "repair_required"
)

type ProjectionReceipt struct {
	Status ProjectionStatus `json:"status"`
	Code   string           `json:"code,omitempty"`
}

type QueryKind string

const (
	QueryObjective       QueryKind = "Objective"
	QueryResume          QueryKind = "Resume"
	QueryLegacyMigration QueryKind = "LegacyMigration"
	QueryLegacyRollback  QueryKind = "LegacyRollback"
)

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

type ResumeView struct {
	Capsule continuity.Capsule `json:"capsule"`
}

func (ResumeView) QueryKind() QueryKind { return QueryResume }
func (ResumeView) isView()              {}

type LegacyMigrationView struct {
	Migration      continuity.LegacyMigration `json:"migration"`
	Committed      bool                       `json:"committed"`
	SwitchPending  bool                       `json:"switch_pending,omitempty"`
	LedgerRevision uint64                     `json:"ledger_revision,omitempty"`
	TransactionID  string                     `json:"transaction_id,omitempty"`
}

func (LegacyMigrationView) QueryKind() QueryKind { return QueryLegacyMigration }
func (LegacyMigrationView) isView()              {}

type LegacyRollbackView struct {
	Rollback continuity.LegacyRollback `json:"rollback"`
}

func (LegacyRollbackView) QueryKind() QueryKind { return QueryLegacyRollback }
func (LegacyRollbackView) isView()              {}

type Engine interface {
	Apply(ctx context.Context, command CommandEnvelope) (Receipt, error)
	Query(ctx context.Context, query Query) (View, error)
}

type kernel struct {
	store      ledger.Store
	continuity *continuity.Module
}

type Option func(*kernel)

func WithContinuity(module *continuity.Module) Option {
	return func(kernel *kernel) { kernel.continuity = module }
}

func New(store ledger.Store, options ...Option) Engine {
	kernel := &kernel{store: store}
	for _, option := range options {
		option(kernel)
	}
	return kernel
}

func (k *kernel) Apply(ctx context.Context, command CommandEnvelope) (receipt Receipt, err error) {
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
	if k.continuity != nil {
		unlock, lockErr := k.continuity.LockLifecycle(ctx)
		if lockErr != nil {
			return Receipt{}, lockErr
		}
		defer func() { err = errors.Join(err, unlock()) }()
	}
	if command.Kind == CommandRollbackLegacyMigration {
		return k.applyLegacyRollback(ctx, command)
	}
	canonicalProject, canonicalFound, err := k.store.Project(ctx)
	if err != nil {
		return Receipt{}, err
	}
	if canonicalFound && canonicalProject != command.ProjectID {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: "PROJECT_CONFLICT", Message: "canonical ledger belongs to another project"}}, nil
	}
	if command.Kind == CommandImportLegacyNative {
		return k.applyLegacyMigration(ctx, command, digest, canonicalFound)
	}
	if receipt, found, err := k.findIdempotentReceipt(ctx, command, digest); err != nil {
		return Receipt{}, err
	} else if found {
		return receipt, nil
	}
	if (command.Kind == CommandStartObjective || command.Kind == CommandSaveObjective) && k.continuity != nil {
		if err := k.continuity.PreflightLifecycle(ctx, command.ProjectID, canonicalFound); err != nil {
			return projectionPreflightRejection(err), nil
		}
		if command.Kind == CommandSaveObjective {
			if _, err := k.continuity.Resume(ctx, continuitySource{store: k.store}); err != nil {
				return projectionPreflightRejection(err), nil
			}
		}
	}
	if command.Kind != CommandStartObjective && command.Kind != CommandSaveObjective {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "UNSUPPORTED_COMMAND",
			Message: fmt.Sprintf("unsupported command kind %q", command.Kind),
		}}, nil
	}
	if command.Actor.Role != ActorUser && command.Actor.Role != ActorCoordinator {
		return Receipt{Accepted: false, Rejection: &Rejection{
			Code:    "FORBIDDEN",
			Message: "only a user or coordinator can update the root objective",
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
	state, err := foldObjectives(transactions)
	if err != nil {
		return Receipt{}, err
	}
	var objective Objective
	var eventKind string
	switch command.Kind {
	case CommandStartObjective:
		if state.activeID != "" {
			return Receipt{Accepted: false, Rejection: &Rejection{
				Code: "OBJECTIVE_EXISTS", Message: "project already has an active root objective",
			}}, nil
		}
		var start StartObjective
		if err := json.Unmarshal(command.Payload, &start); err != nil {
			return invalidCommandReceipt(fmt.Sprintf("decode StartObjective: %v", err)), nil
		}
		defaultStartObjective(&start)
		if err := validateStartObjective(start); err != nil {
			return invalidCommandReceipt(err.Error()), nil
		}
		secretInputs := append([]string{start.Title, start.Goal, start.Profile}, start.Acceptance...)
		secretInputs = append(secretInputs, start.Next...)
		if containsHighConfidenceSecret(secretInputs...) {
			return Receipt{Accepted: false, Rejection: &Rejection{Code: "SENSITIVE_CONTENT", Message: "objective contains a high-confidence secret pattern and was not written"}}, nil
		}
		objectiveID, idErr := newID("obj")
		if idErr != nil {
			return Receipt{}, idErr
		}
		objective = Objective{
			ObjectiveID: objectiveID, ProjectID: command.ProjectID,
			Title: strings.TrimSpace(start.Title), Goal: strings.TrimSpace(start.Goal),
			Acceptance: cleanStrings(start.Acceptance), Profile: strings.TrimSpace(start.Profile),
			Status: ObjectiveActive, Revision: 1,
			Done: []string{}, Next: cleanStrings(start.Next), Validation: []string{}, Blockers: []string{}, MustRead: []string{},
		}
		eventKind = "ObjectiveStarted"
	case CommandSaveObjective:
		var save SaveObjective
		if err := json.Unmarshal(command.Payload, &save); err != nil {
			return invalidCommandReceipt(fmt.Sprintf("decode SaveObjective: %v", err)), nil
		}
		if err := validateSaveObjective(save); err != nil {
			return invalidCommandReceipt(err.Error()), nil
		}
		secretInputs := append(append(append([]string{save.ObjectiveID}, save.Done...), save.Validation...), valuesFromPointers(save.Next, save.Blockers, save.MustRead)...)
		if containsHighConfidenceSecret(secretInputs...) {
			return Receipt{Accepted: false, Rejection: &Rejection{Code: "SENSITIVE_CONTENT", Message: "checkpoint contains a high-confidence secret pattern and was not written"}}, nil
		}
		if state.activeID == "" {
			return Receipt{Accepted: false, Rejection: &Rejection{Code: "NO_ACTIVE_OBJECTIVE", Message: "project has no active root objective"}}, nil
		}
		if state.activeID != save.ObjectiveID {
			return Receipt{Accepted: false, Rejection: &Rejection{Code: "OBJECTIVE_NOT_CURRENT", Message: "save targets a root objective that is not current"}}, nil
		}
		current := state.objectives[state.activeID]
		if current.Revision != save.ExpectedObjectiveRevision {
			return Receipt{Accepted: false, Rejection: &Rejection{Code: "OBJECTIVE_REVISION_CONFLICT", Message: "expected objective revision does not match current state"}}, nil
		}
		objective, err = applyCheckpoint(current, save)
		if err != nil {
			return invalidCommandReceipt(err.Error()), nil
		}
		eventKind = "ObjectiveSaved"
	}
	if k.continuity != nil {
		preflightState := continuityState(objective, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC))
		resumeDigest, digestErr := continuity.CanonicalResumeDigest(preflightState)
		if digestErr != nil {
			return Receipt{}, fmt.Errorf("digest continuity state: %w", digestErr)
		}
		preflightCursor := continuity.Cursor{Revision: command.ExpectedRevision + 1, Digest: strings.Repeat("0", sha256.Size*2), ResumeDigest: resumeDigest}
		if err := k.continuity.PreflightStart(ctx, preflightState, preflightCursor); err != nil {
			return projectionPreflightRejection(err), nil
		}
	}
	resumeDigest, err := continuity.CanonicalResumeDigest(continuityState(objective, time.Time{}))
	if err != nil {
		return Receipt{}, fmt.Errorf("digest continuity state: %w", err)
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
		ResumeDigest:   resumeDigest,
		IdempotencyKey: command.IdempotencyKey,
		CorrelationID:  command.CorrelationID,
		CausationID:    command.CausationID,
		IssuedAt:       command.IssuedAt,
		Actor:          actor,
		Events: []ledger.Event{{
			EventID:  eventID,
			Kind:     eventKind,
			EntityID: objective.ObjectiveID,
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
	return k.receiptWithProjection(ctx, transaction), nil
}

func invalidCommandReceipt(message string) Receipt {
	return Receipt{Accepted: false, Rejection: &Rejection{Code: "INVALID_COMMAND", Message: message}}
}

func projectionPreflightRejection(err error) Receipt {
	code := continuity.ErrorCode(err)
	if code == "" {
		code = continuity.CodeProjectionConflict
	}
	return Receipt{Accepted: false, Rejection: &Rejection{Code: string(code), Message: err.Error()}}
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
	return k.receiptWithProjection(ctx, transaction), true, nil
}

func (k *kernel) receiptWithProjection(ctx context.Context, transaction ledger.Transaction) Receipt {
	receipt := receiptFromTransaction(transaction)
	if k.continuity == nil {
		return receipt
	}
	source := continuitySource{store: k.store}
	state, cursor, err := source.Snapshot(ctx, transaction.ProjectID)
	if err != nil {
		code := continuity.ErrorCode(err)
		if code == "" {
			code = continuity.CodeProjectionConflict
		}
		receipt.Projection = &ProjectionReceipt{Status: ProjectionRepairRequired, Code: string(code)}
		return receipt
	}
	if _, err := k.continuity.Project(ctx, state, cursor); err != nil {
		code := continuity.ErrorCode(err)
		if code == "" {
			code = continuity.CodeProjectionConflict
		}
		receipt.Projection = &ProjectionReceipt{Status: ProjectionRepairRequired, Code: string(code)}
		return receipt
	}
	head, err := k.store.Head(ctx, transaction.ProjectID)
	if err != nil || head.Revision != cursor.Revision || head.Digest != cursor.Digest || head.ResumeDigest != cursor.ResumeDigest {
		receipt.Projection = &ProjectionReceipt{Status: ProjectionRepairRequired, Code: string(continuity.CodeProjectionStale)}
		return receipt
	}
	receipt.Projection = &ProjectionReceipt{Status: ProjectionCurrent}
	return receipt
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
		if event.Kind == "ObjectiveStarted" || event.Kind == "ObjectiveSaved" {
			var objective Objective
			if json.Unmarshal(event.Data, &objective) == nil {
				receipt.EntityRevision = objective.Revision
				receipt.EntityStatus = string(objective.Status)
			}
		}
	}
	return receipt
}

func (k *kernel) Query(ctx context.Context, query Query) (View, error) {
	if query.Kind == QueryLegacyRollback {
		if k.continuity == nil {
			return nil, &continuity.Error{Code: continuity.CodeCapabilityUnavailable, Op: "query legacy rollback", Err: errors.New("continuity module is not configured")}
		}
		rollback, err := k.continuity.InspectLegacyRollback(ctx)
		if err != nil {
			return nil, err
		}
		return LegacyRollbackView{Rollback: rollback}, nil
	}
	if query.Kind == QueryLegacyMigration {
		if k.continuity == nil {
			return nil, &continuity.Error{Code: continuity.CodeCapabilityUnavailable, Op: "query legacy migration", Err: errors.New("continuity module is not configured")}
		}
		projectID, found, err := k.store.Project(ctx)
		if err != nil {
			return nil, err
		}
		if !found {
			migration, inspectErr := k.continuity.InspectLegacyNative(ctx)
			if inspectErr != nil {
				return nil, inspectErr
			}
			events, active, buildErr := buildLegacyMigrationEvents(migration)
			if buildErr != nil {
				return nil, migrationQueryError(buildErr)
			}
			if validateErr := validateLegacyMigrationDraft(migration.ProjectID, events); validateErr != nil {
				return nil, migrationQueryError(validateErr)
			}
			resumeDigest, digestErr := continuity.CanonicalResumeDigest(continuityState(active, time.Time{}))
			if digestErr != nil {
				return nil, digestErr
			}
			preflightState := continuityState(active, time.Unix(1, 0).UTC())
			if preflightErr := k.continuity.PreflightStart(ctx, preflightState, continuity.Cursor{Revision: 1, Digest: strings.Repeat("0", sha256.Size*2), ResumeDigest: resumeDigest}); preflightErr != nil {
				return nil, preflightErr
			}
			return LegacyMigrationView{Migration: migration}, nil
		}
		if query.ProjectID != "" && query.ProjectID != projectID {
			return nil, fmt.Errorf("canonical ledger belongs to project %q, requested %q", projectID, query.ProjectID)
		}
		transactions, transactionErr := k.store.Transactions(ctx, projectID)
		if transactionErr != nil {
			return nil, transactionErr
		}
		migration, transactionID, foldErr := foldLegacyMigration(projectID, transactions)
		if foldErr != nil {
			return nil, foldErr
		}
		switchPending, switchErr := k.continuity.LegacySwitchPending(ctx, migration.HandoffDigest)
		if switchErr != nil {
			return nil, switchErr
		}
		return LegacyMigrationView{Migration: migration, Committed: true, SwitchPending: switchPending, LedgerRevision: transactions[len(transactions)-1].Revision, TransactionID: transactionID}, nil
	}
	if query.Kind == QueryResume {
		if k.continuity == nil {
			return nil, &continuity.Error{Code: continuity.CodeCapabilityUnavailable, Op: "query resume", Err: errors.New("continuity module is not configured")}
		}
		capsule, err := k.continuity.Resume(ctx, continuitySource{store: k.store})
		if err != nil {
			return nil, err
		}
		return ResumeView{Capsule: capsule}, nil
	}
	if query.Kind != QueryObjective {
		return nil, fmt.Errorf("unsupported query kind %q", query.Kind)
	}
	transactions, err := k.store.Transactions(ctx, query.ProjectID)
	if err != nil {
		return nil, err
	}
	state, err := foldObjectives(transactions)
	if err != nil {
		return nil, err
	}
	objective, found := state.objectives[query.EntityID]
	if found {
		return ObjectiveView{Objective: objective}, nil
	}
	return nil, fmt.Errorf("objective %q not found", query.EntityID)
}

type continuitySource struct {
	store ledger.Store
}

func (source continuitySource) Project(ctx context.Context) (string, bool, error) {
	return source.store.Project(ctx)
}

func (source continuitySource) Snapshot(ctx context.Context, projectID string) (continuity.State, continuity.Cursor, error) {
	transactions, err := source.store.Transactions(ctx, projectID)
	if err != nil {
		return continuity.State{}, continuity.Cursor{}, err
	}
	if len(transactions) == 0 {
		return continuity.State{}, continuity.Cursor{}, errors.New("canonical ledger has no objective")
	}
	state, err := foldObjectives(transactions)
	if err != nil {
		return continuity.State{}, continuity.Cursor{}, err
	}
	if state.activeID == "" {
		return continuity.State{}, continuity.Cursor{}, errors.New("canonical ledger has no root objective")
	}
	objective := state.objectives[state.activeID]
	last := transactions[len(transactions)-1]
	return continuityState(objective, last.CommittedAt), continuity.Cursor{Revision: last.Revision, Digest: last.Digest, ResumeDigest: last.ResumeDigest}, nil
}

func (source continuitySource) Head(ctx context.Context, projectID string) (continuity.Cursor, error) {
	transactions, err := source.store.Transactions(ctx, projectID)
	if err != nil {
		return continuity.Cursor{}, err
	}
	if len(transactions) == 0 {
		return continuity.Cursor{}, nil
	}
	last := transactions[len(transactions)-1]
	return continuity.Cursor{Revision: last.Revision, Digest: last.Digest, ResumeDigest: last.ResumeDigest}, nil
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
	if err := validateValues(start.Next, "next item", maxNextItems); err != nil {
		return err
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

func defaultStartObjective(start *StartObjective) {
	start.Goal = strings.TrimSpace(start.Goal)
	if strings.TrimSpace(start.Title) == "" {
		start.Title = start.Goal
	}
	if len(cleanStrings(start.Acceptance)) == 0 && start.Goal != "" {
		start.Acceptance = []string{start.Goal}
	}
	if len(cleanStrings(start.Next)) == 0 && start.Goal != "" {
		start.Next = []string{start.Goal}
	}
	if strings.TrimSpace(start.Profile) == "" {
		start.Profile = "standard"
	}
}

func validateBoundedText(value, label string, maxChars int) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single line", label)
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
