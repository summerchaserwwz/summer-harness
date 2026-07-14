package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/ledger"
)

const legacyMigrationEventSchema = "summer.legacy-import/v1"

type legacyImportedRecord struct {
	Schema         string          `json:"schema"`
	MigrationID    string          `json:"migration_id"`
	LegacyEntityID string          `json:"legacy_entity_id"`
	LegacyTaskID   string          `json:"legacy_task_id"`
	ObjectiveID    string          `json:"objective_id"`
	SourcePath     string          `json:"source_path"`
	SourceDigest   string          `json:"source_digest"`
	SourceLine     int             `json:"source_line,omitempty"`
	Objective      *Objective      `json:"objective,omitempty"`
	Record         json.RawMessage `json:"record"`
}

type legacyMigrationCompleted struct {
	Schema               string                           `json:"schema"`
	MigrationID          string                           `json:"migration_id"`
	ProjectID            string                           `json:"project_id"`
	SourceDigest         string                           `json:"source_digest"`
	SemanticDigest       string                           `json:"semantic_digest"`
	BackupManifestDigest string                           `json:"backup_manifest_digest"`
	HandoffDigest        string                           `json:"handoff_digest"`
	ActiveLegacyTaskID   string                           `json:"active_legacy_task_id"`
	ActiveObjectiveID    string                           `json:"active_objective_id"`
	Counts               continuity.LegacyMigrationCounts `json:"counts"`
}

func (k *kernel) applyLegacyMigration(ctx context.Context, command CommandEnvelope, commandDigest string, canonicalFound bool) (Receipt, error) {
	if k.continuity == nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeCapabilityUnavailable), Message: "legacy migration requires the continuity module"}}, nil
	}
	if command.Actor.Role != ActorUser {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: "FORBIDDEN", Message: "only a user can migrate a legacy lifecycle"}}, nil
	}
	if command.ExpectedRevision != 0 {
		return invalidCommandReceipt("legacy migration requires expected revision 0"), nil
	}
	var request ImportLegacyNative
	if err := decodeStrictPayload(command.Payload, &request); err != nil {
		return invalidCommandReceipt(fmt.Sprintf("decode ImportLegacyNative: %v", err)), nil
	}
	if err := validateLegacyMigrationRequest(request); err != nil {
		return invalidCommandReceipt(err.Error()), nil
	}
	if canonicalFound {
		return k.resumeCommittedLegacyMigration(ctx, command, request)
	}

	plan, err := k.continuity.InspectLegacyNative(ctx)
	if err != nil {
		return projectionPreflightRejection(err), nil
	}
	if err := compareMigrationRequest(command.ProjectID, request, plan); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationSourceChanged), Message: err.Error()}}, nil
	}
	if err := k.continuity.BackupLegacy(ctx, plan); err != nil {
		return projectionPreflightRejection(err), nil
	}
	if err := k.continuity.VerifyLegacySource(ctx, plan); err != nil {
		return projectionPreflightRejection(err), nil
	}
	events, active, err := buildLegacyMigrationEvents(plan)
	if err != nil {
		return migrationBuildRejection(err), nil
	}
	if err := validateLegacyMigrationDraft(plan.ProjectID, events); err != nil {
		return migrationBuildRejection(err), nil
	}
	state := continuityState(active, command.IssuedAt)
	resumeDigest, err := continuity.CanonicalResumeDigest(continuityState(active, zeroTime))
	if err != nil {
		return Receipt{}, fmt.Errorf("digest imported continuity state: %w", err)
	}
	preflightCursor := continuity.Cursor{Revision: 1, Digest: strings.Repeat("0", sha256.Size*2), ResumeDigest: resumeDigest}
	if err := k.continuity.PreflightStart(ctx, state, preflightCursor); err != nil {
		return projectionPreflightRejection(err), nil
	}
	actor, err := json.Marshal(command.Actor)
	if err != nil {
		return Receipt{}, err
	}
	transactionID, err := newID("tx")
	if err != nil {
		return Receipt{}, err
	}
	transaction, err := k.store.Commit(ctx, ledger.Draft{
		TransactionID: transactionID, ProjectID: command.ProjectID,
		CommandID: command.CommandID, CommandDigest: commandDigest, ResumeDigest: resumeDigest,
		IdempotencyKey: command.IdempotencyKey, CorrelationID: command.CorrelationID,
		CausationID: command.CausationID, IssuedAt: command.IssuedAt, Actor: actor, Events: events,
	}, 0)
	if errors.Is(err, ledger.ErrRevisionConflict) {
		return k.resumeCommittedLegacyMigration(ctx, command, request)
	}
	if errors.Is(err, ledger.ErrIdempotencyConflict) {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key was already used for a different command"}}, nil
	}
	if err != nil {
		return Receipt{}, err
	}
	if err := k.verifyAndProjectLegacyMigration(ctx, plan, transaction); err != nil {
		return Receipt{Accepted: false, TransactionID: transaction.TransactionID, NewRevision: transaction.Revision, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: err.Error()}}, nil
	}
	receipt := receiptFromTransaction(transaction)
	receipt.EntityID = plan.ActiveObjectiveID
	receipt.EntityRevision = active.Revision
	receipt.EntityStatus = string(active.Status)
	receipt.Projection = &ProjectionReceipt{Status: ProjectionCurrent}
	return receipt, nil
}

func (k *kernel) applyLegacyRollback(ctx context.Context, command CommandEnvelope) (Receipt, error) {
	if k.continuity == nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeCapabilityUnavailable), Message: "legacy rollback requires the continuity module"}}, nil
	}
	if command.Actor.Role != ActorUser {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: "FORBIDDEN", Message: "only a user can roll back a legacy migration"}}, nil
	}
	var request RollbackLegacyMigration
	if err := decodeStrictPayload(command.Payload, &request); err != nil {
		return invalidCommandReceipt(fmt.Sprintf("decode RollbackLegacyMigration: %v", err)), nil
	}
	if !strings.HasPrefix(request.MigrationID, "migration_") || request.ExpectedTransactionID == "" || !validSHA256(request.ExpectedLedgerHead) {
		return invalidCommandReceipt("rollback migration id, transaction id, or ledger head is invalid"), nil
	}
	rollback, err := k.continuity.InspectLegacyRollback(ctx)
	if err != nil {
		return projectionPreflightRejection(err), nil
	}
	if rollback.ProjectID != command.ProjectID || rollback.MigrationID != request.MigrationID || rollback.TransactionID != request.ExpectedTransactionID || rollback.LedgerHead != request.ExpectedLedgerHead {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeRollbackSourceDrift), Message: "rollback request does not match the migration genesis and backup"}}, nil
	}
	ref := ledger.GenesisRef{ProjectID: rollback.ProjectID, TransactionID: rollback.TransactionID, Digest: rollback.LedgerHead}
	quarantiner, ok := k.store.(ledger.GenesisQuarantiner)
	if !ok {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeCapabilityUnavailable), Message: "canonical store does not support migration rollback"}}, nil
	}
	genesis, loadErr := quarantiner.LoadGenesis(ctx, ref, rollback.MigrationID)
	if loadErr != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeRollbackNotAllowed), Message: loadErr.Error()}}, nil
	}
	canonical, transactionID, foldErr := foldLegacyMigration(rollback.ProjectID, []ledger.Transaction{genesis})
	if foldErr != nil || transactionID != rollback.TransactionID || canonical.MigrationID != rollback.MigrationID {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeRollbackNotAllowed), Message: "rollback genesis is not a complete legacy migration"}}, nil
	}
	if canonical.SourceDigest != rollback.SourceDigest || canonical.SemanticDigest != rollback.SemanticDigest || canonical.BackupManifestDigest != rollback.BackupManifestDigest || canonical.HandoffDigest != rollback.HandoffDigest {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeRollbackSourceDrift), Message: "migration backup no longer matches the canonical genesis"}}, nil
	}
	if err := k.continuity.PreflightLegacyRollback(ctx, rollback); err != nil {
		return projectionPreflightRejection(err), nil
	}
	if err := quarantiner.QuarantineGenesis(ctx, ref, rollback.MigrationID); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeRollbackNotAllowed), Message: err.Error()}}, nil
	}
	if err := k.continuity.RestoreLegacyBackup(ctx, rollback); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: err.Error()}}, nil
	}
	if err := quarantiner.CompleteGenesisQuarantine(ctx, ref, rollback.MigrationID); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: err.Error()}}, nil
	}
	return Receipt{Accepted: true, EntityID: rollback.MigrationID, EntityStatus: "rolled_back", EmittedEventIDs: []string{}}, nil
}

var zeroTime = func() (value time.Time) { return }()

func (k *kernel) resumeCommittedLegacyMigration(ctx context.Context, command CommandEnvelope, request ImportLegacyNative) (Receipt, error) {
	transactions, err := k.store.Transactions(ctx, command.ProjectID)
	if err != nil {
		return Receipt{}, err
	}
	plan, transactionID, err := foldLegacyMigration(command.ProjectID, transactions)
	if err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: err.Error()}}, nil
	}
	if err := compareMigrationRequest(command.ProjectID, request, plan); err != nil {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationSourceChanged), Message: err.Error()}}, nil
	}
	if len(transactions) != 1 || transactions[0].TransactionID != transactionID {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: "legacy handoff cannot be switched after later canonical transactions"}}, nil
	}
	if err := k.verifyAndProjectLegacyMigration(ctx, plan, transactions[0]); err != nil {
		return Receipt{Accepted: false, TransactionID: transactionID, NewRevision: 1, Rejection: &Rejection{Code: string(continuity.CodeMigrationIncomplete), Message: err.Error()}}, nil
	}
	state, _, err := foldActiveImportedObjective(transactions)
	if err != nil {
		return Receipt{}, err
	}
	receipt := receiptFromTransaction(transactions[0])
	receipt.EntityID = state.ObjectiveID
	receipt.EntityRevision = state.Revision
	receipt.EntityStatus = string(state.Status)
	receipt.Projection = &ProjectionReceipt{Status: ProjectionCurrent}
	return receipt, nil
}

func (k *kernel) verifyAndProjectLegacyMigration(ctx context.Context, expected continuity.LegacyMigration, transaction ledger.Transaction) error {
	transactions, err := k.store.Transactions(ctx, expected.ProjectID)
	if err != nil {
		return err
	}
	actual, transactionID, err := foldLegacyMigration(expected.ProjectID, transactions)
	if err != nil {
		return err
	}
	if transactionID != transaction.TransactionID {
		return errors.New("canonical migration transaction does not match the committed import")
	}
	if difference := legacyMigrationDifference(expected, actual); difference != "" {
		return fmt.Errorf("canonical migration does not reproduce %s", difference)
	}
	pending, err := k.continuity.LegacySwitchPending(ctx, expected.HandoffDigest)
	if err != nil {
		return err
	}
	if pending {
		live, inspectErr := k.continuity.InspectLegacyNative(ctx)
		if inspectErr != nil {
			return inspectErr
		}
		if difference := legacyMigrationDifference(expected, live); difference != "" {
			return fmt.Errorf("live legacy source no longer reproduces canonical %s", difference)
		}
		expected = live
		if err := k.continuity.VerifyLegacySource(ctx, live); err != nil {
			return err
		}
	}
	active, state, err := foldActiveImportedObjective(transactions)
	if err != nil {
		return err
	}
	head, err := k.store.Head(ctx, expected.ProjectID)
	if err != nil {
		return err
	}
	cursor := continuity.Cursor{Revision: head.Revision, Digest: head.Digest, ResumeDigest: head.ResumeDigest}
	state.BuiltAt = transaction.CommittedAt
	if _, err := k.continuity.ProjectMigrated(ctx, state, cursor, expected.HandoffDigest); err != nil {
		return err
	}
	if active.ObjectiveID != expected.ActiveObjectiveID {
		return errors.New("canonical active objective differs from the migration summary")
	}
	return nil
}

func buildLegacyMigrationEvents(plan continuity.LegacyMigration) ([]ledger.Event, Objective, error) {
	objectiveByID := make(map[string]Objective, len(plan.Objectives))
	var active Objective
	for _, imported := range plan.Objectives {
		objective := Objective{
			ObjectiveID: imported.ObjectiveID, ProjectID: plan.ProjectID, Title: strings.TrimSpace(imported.Title),
			Goal: strings.TrimSpace(imported.Goal), Acceptance: cleanStrings(imported.Acceptance), Profile: strings.TrimSpace(imported.Profile), Risk: strings.TrimSpace(imported.Risk),
			Status: ObjectiveStatus(imported.Status), Revision: imported.Revision,
			Done: cleanStrings(imported.Done), Next: cleanStrings(imported.Next), Validation: cleanStrings(imported.Validation),
			Blockers: cleanStrings(imported.Blockers), MustRead: cleanStrings(imported.MustRead), ResidualRisks: cleanStrings(imported.ResidualRisks),
		}
		if objective.ObjectiveID == "" || objective.ProjectID == "" || objective.Title == "" || objective.Goal == "" || len(objective.Acceptance) == 0 || objective.Profile == "" || objective.Revision == 0 || !validObjectiveStatus(objective.Status) {
			return nil, Objective{}, errors.New("legacy objective cannot be represented by the canonical model")
		}
		objectiveByID[objective.ObjectiveID] = objective
		if objective.ObjectiveID == plan.ActiveObjectiveID {
			active = objective
		}
	}
	if active.ObjectiveID == "" || !isNonterminalObjectiveStatus(active.Status) {
		return nil, Objective{}, errors.New("legacy migration has no nonterminal active objective")
	}
	events := make([]ledger.Event, 0, len(plan.Records)+1)
	for index, record := range plan.Records {
		payload := legacyImportedRecord{
			Schema: legacyMigrationEventSchema, MigrationID: plan.MigrationID,
			LegacyEntityID: record.EntityID, LegacyTaskID: record.LegacyTaskID, ObjectiveID: record.ObjectiveID,
			SourcePath: record.SourcePath, SourceDigest: record.SourceDigest, SourceLine: record.SourceLine,
			Record: append(json.RawMessage(nil), record.Data...),
		}
		entityID := record.EntityID
		if record.EventKind == "ObjectiveImported" {
			objective, exists := objectiveByID[record.ObjectiveID]
			if !exists {
				return nil, Objective{}, fmt.Errorf("objective import %q has no canonical objective", record.LegacyTaskID)
			}
			payload.Objective = &objective
			entityID = objective.ObjectiveID
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, Objective{}, err
		}
		if containsHighConfidenceSecret(string(data)) {
			return nil, Objective{}, errors.New("imported event contains a high-confidence secret pattern")
		}
		events = append(events, ledger.Event{EventID: deterministicMigrationEventID(plan.MigrationID, index, record.EventKind, entityID), Kind: record.EventKind, EntityID: entityID, Data: data})
	}
	completed := legacyMigrationCompleted{
		Schema: legacyMigrationEventSchema, MigrationID: plan.MigrationID, ProjectID: plan.ProjectID,
		SourceDigest: plan.SourceDigest, SemanticDigest: plan.SemanticDigest, BackupManifestDigest: plan.BackupManifestDigest,
		HandoffDigest: plan.HandoffDigest, ActiveLegacyTaskID: plan.ActiveLegacyTaskID, ActiveObjectiveID: plan.ActiveObjectiveID,
		Counts: plan.Counts,
	}
	data, err := json.Marshal(completed)
	if err != nil {
		return nil, Objective{}, err
	}
	events = append(events, ledger.Event{EventID: deterministicMigrationEventID(plan.MigrationID, len(events), "LegacyMigrationCompleted", plan.MigrationID), Kind: "LegacyMigrationCompleted", EntityID: plan.MigrationID, Data: data})
	return events, active, nil
}

func foldLegacyMigration(projectID string, transactions []ledger.Transaction) (continuity.LegacyMigration, string, error) {
	if len(transactions) == 0 {
		return continuity.LegacyMigration{}, "", errors.New("canonical ledger has no migration transaction")
	}
	plan := continuity.LegacyMigration{ProjectID: projectID}
	transactionID := ""
	completedSeen := false
	for _, transaction := range transactions {
		for _, event := range transaction.Events {
			switch event.Kind {
			case "ObjectiveImported", "DecisionRecorded", "FactRecorded", "FactInvalidated":
				var imported legacyImportedRecord
				if err := decodeStrictPayload(event.Data, &imported); err != nil {
					return continuity.LegacyMigration{}, "", fmt.Errorf("decode %s: %w", event.Kind, err)
				}
				if imported.Schema != legacyMigrationEventSchema || imported.MigrationID == "" || imported.LegacyEntityID == "" || imported.LegacyTaskID == "" || imported.ObjectiveID == "" || imported.SourcePath == "" || imported.SourceDigest == "" || len(imported.Record) == 0 {
					return continuity.LegacyMigration{}, "", fmt.Errorf("%s event is incomplete", event.Kind)
				}
				if plan.MigrationID == "" {
					plan.MigrationID = imported.MigrationID
				} else if plan.MigrationID != imported.MigrationID {
					return continuity.LegacyMigration{}, "", errors.New("canonical ledger contains multiple legacy migrations")
				}
				if event.Kind == "ObjectiveImported" {
					if imported.Objective == nil || event.EntityID != imported.Objective.ObjectiveID || imported.Objective.ProjectID != projectID {
						return continuity.LegacyMigration{}, "", errors.New("ObjectiveImported event has inconsistent canonical identity")
					}
					plan.Objectives = append(plan.Objectives, legacyObjectiveFromCanonical(*imported.Objective, imported.LegacyTaskID))
				} else if event.EntityID != imported.LegacyEntityID {
					return continuity.LegacyMigration{}, "", fmt.Errorf("%s event has inconsistent legacy identity", event.Kind)
				}
				plan.Records = append(plan.Records, continuity.LegacyMigrationRecord{
					EventKind: event.Kind, EntityID: imported.LegacyEntityID, LegacyTaskID: imported.LegacyTaskID,
					ObjectiveID: imported.ObjectiveID, SourcePath: imported.SourcePath, SourceDigest: imported.SourceDigest,
					SourceLine: imported.SourceLine, Data: append(json.RawMessage(nil), imported.Record...),
				})
			case "LegacyMigrationCompleted":
				if completedSeen {
					return continuity.LegacyMigration{}, "", errors.New("canonical ledger completes legacy migration more than once")
				}
				var completed legacyMigrationCompleted
				if err := decodeStrictPayload(event.Data, &completed); err != nil {
					return continuity.LegacyMigration{}, "", err
				}
				if completed.Schema != legacyMigrationEventSchema || completed.MigrationID == "" || completed.ProjectID != projectID || event.EntityID != completed.MigrationID {
					return continuity.LegacyMigration{}, "", errors.New("LegacyMigrationCompleted event is incomplete")
				}
				if plan.MigrationID != "" && plan.MigrationID != completed.MigrationID {
					return continuity.LegacyMigration{}, "", errors.New("legacy migration summary does not match imported records")
				}
				plan.MigrationID = completed.MigrationID
				plan.SourceDigest = completed.SourceDigest
				plan.SemanticDigest = completed.SemanticDigest
				plan.BackupManifestDigest = completed.BackupManifestDigest
				plan.HandoffDigest = completed.HandoffDigest
				plan.ActiveLegacyTaskID = completed.ActiveLegacyTaskID
				plan.ActiveObjectiveID = completed.ActiveObjectiveID
				plan.Counts = completed.Counts
				transactionID = transaction.TransactionID
				completedSeen = true
			}
		}
	}
	if !completedSeen || plan.MigrationID == "" {
		return continuity.LegacyMigration{}, "", errors.New("canonical ledger has no completed legacy migration")
	}
	semanticDigest, err := legacySemanticDigest(plan)
	if err != nil {
		return continuity.LegacyMigration{}, "", err
	}
	actualCounts := countLegacyRecords(plan)
	if semanticDigest != plan.SemanticDigest || !reflect.DeepEqual(actualCounts, plan.Counts) {
		return continuity.LegacyMigration{}, "", errors.New("canonical legacy migration summary does not match its records")
	}
	return plan, transactionID, nil
}

func foldActiveImportedObjective(transactions []ledger.Transaction) (Objective, continuity.State, error) {
	state, err := foldObjectives(transactions)
	if err != nil {
		return Objective{}, continuity.State{}, err
	}
	if state.activeID == "" {
		return Objective{}, continuity.State{}, errors.New("canonical migration has no nonterminal root objective")
	}
	objective := state.objectives[state.activeID]
	return objective, continuityState(objective, zeroTime), nil
}

func legacyObjectiveFromCanonical(objective Objective, legacyTaskID string) continuity.LegacyObjectiveImport {
	return continuity.LegacyObjectiveImport{
		ObjectiveID: objective.ObjectiveID, LegacyTaskID: legacyTaskID, Title: objective.Title, Goal: objective.Goal,
		Acceptance: cloneStrings(objective.Acceptance), Profile: objective.Profile, Risk: objective.Risk, Status: string(objective.Status), Revision: objective.Revision,
		Done: cloneStrings(objective.Done), Next: cloneStrings(objective.Next), Validation: cloneStrings(objective.Validation),
		Blockers: cloneStrings(objective.Blockers), MustRead: cloneStrings(objective.MustRead), ResidualRisks: cloneStrings(objective.ResidualRisks),
	}
}

func equivalentLegacyMigration(expected, actual continuity.LegacyMigration) bool {
	return expected.MigrationID == actual.MigrationID && expected.ProjectID == actual.ProjectID &&
		expected.SourceDigest == actual.SourceDigest && expected.SemanticDigest == actual.SemanticDigest &&
		expected.BackupManifestDigest == actual.BackupManifestDigest && expected.HandoffDigest == actual.HandoffDigest &&
		expected.ActiveLegacyTaskID == actual.ActiveLegacyTaskID && expected.ActiveObjectiveID == actual.ActiveObjectiveID &&
		reflect.DeepEqual(expected.Counts, actual.Counts) && reflect.DeepEqual(expected.Objectives, actual.Objectives) &&
		reflect.DeepEqual(expected.Records, actual.Records)
}

func legacyMigrationDifference(expected, actual continuity.LegacyMigration) string {
	for label, values := range map[string][2]string{
		"migration id": {expected.MigrationID, actual.MigrationID}, "project id": {expected.ProjectID, actual.ProjectID},
		"source digest": {expected.SourceDigest, actual.SourceDigest}, "semantic digest": {expected.SemanticDigest, actual.SemanticDigest},
		"backup manifest digest": {expected.BackupManifestDigest, actual.BackupManifestDigest}, "handoff digest": {expected.HandoffDigest, actual.HandoffDigest},
		"active legacy task": {expected.ActiveLegacyTaskID, actual.ActiveLegacyTaskID}, "active objective": {expected.ActiveObjectiveID, actual.ActiveObjectiveID},
	} {
		if values[0] != values[1] {
			return label
		}
	}
	if !reflect.DeepEqual(expected.Counts, actual.Counts) {
		return "entity counts"
	}
	if !reflect.DeepEqual(expected.Objectives, actual.Objectives) {
		return "objective history"
	}
	if !legacyRecordsEquivalent(expected.Records, actual.Records) {
		if len(expected.Records) != len(actual.Records) {
			return fmt.Sprintf("imported record count (%d != %d)", len(expected.Records), len(actual.Records))
		}
		for index := range expected.Records {
			if !legacyRecordEquivalent(expected.Records[index], actual.Records[index]) {
				left, right := expected.Records[index], actual.Records[index]
				field := "data"
				switch {
				case left.EventKind != right.EventKind:
					field = "event kind"
				case left.EntityID != right.EntityID:
					field = "entity id"
				case left.LegacyTaskID != right.LegacyTaskID:
					field = "legacy task id"
				case left.ObjectiveID != right.ObjectiveID:
					field = "objective id"
				case left.SourcePath != right.SourcePath:
					field = "source path"
				case left.SourceDigest != right.SourceDigest:
					field = "source digest"
				case left.SourceLine != right.SourceLine:
					field = "source line"
				}
				return fmt.Sprintf("imported record %d %s", index, field)
			}
		}
		return "imported records"
	}
	return ""
}

func legacyRecordsEquivalent(left, right []continuity.LegacyMigrationRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !legacyRecordEquivalent(left[index], right[index]) {
			return false
		}
	}
	return true
}

func legacyRecordEquivalent(left, right continuity.LegacyMigrationRecord) bool {
	if left.EventKind != right.EventKind || left.EntityID != right.EntityID || left.LegacyTaskID != right.LegacyTaskID || left.ObjectiveID != right.ObjectiveID || left.SourcePath != right.SourcePath || left.SourceDigest != right.SourceDigest || left.SourceLine != right.SourceLine {
		return false
	}
	var leftCompact, rightCompact bytes.Buffer
	if json.Compact(&leftCompact, left.Data) != nil || json.Compact(&rightCompact, right.Data) != nil {
		return false
	}
	return bytes.Equal(leftCompact.Bytes(), rightCompact.Bytes())
}

func legacySemanticDigest(plan continuity.LegacyMigration) (string, error) {
	raw, err := json.Marshal(struct {
		Objectives []continuity.LegacyObjectiveImport `json:"objectives"`
		Records    []continuity.LegacyMigrationRecord `json:"records"`
	}{plan.Objectives, plan.Records})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func countLegacyRecords(plan continuity.LegacyMigration) continuity.LegacyMigrationCounts {
	counts := continuity.LegacyMigrationCounts{Objectives: len(plan.Objectives)}
	for _, record := range plan.Records {
		switch record.EventKind {
		case "DecisionRecorded":
			counts.Decisions++
		case "FactRecorded":
			counts.Facts++
		case "FactInvalidated":
			counts.FactInvalidations++
		}
	}
	return counts
}

func validateLegacyMigrationRequest(request ImportLegacyNative) error {
	for label, value := range map[string]string{"migration_id": request.MigrationID, "source_digest": request.SourceDigest, "backup_manifest_digest": request.BackupManifestDigest} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
	}
	if !strings.HasPrefix(request.MigrationID, "migration_") || !validSHA256(request.SourceDigest) || !validSHA256(request.BackupManifestDigest) {
		return errors.New("legacy migration identifiers or digests are invalid")
	}
	return nil
}

func compareMigrationRequest(projectID string, request ImportLegacyNative, plan continuity.LegacyMigration) error {
	if projectID != plan.ProjectID || request.MigrationID != plan.MigrationID || request.SourceDigest != plan.SourceDigest || request.BackupManifestDigest != plan.BackupManifestDigest {
		return errors.New("migration request does not match the fully inspected legacy source")
	}
	return nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func deterministicMigrationEventID(migrationID string, index int, kind, entityID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%06d:%s:%s", migrationID, index, kind, entityID)))
	return "evt_" + hex.EncodeToString(sum[:12])
}

func decodeStrictPayload(raw []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
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

func migrationBuildRejection(err error) Receipt {
	code := continuity.CodeMigrationVerifyFailed
	if strings.Contains(err.Error(), "limit") || strings.Contains(err.Error(), "exceeds") || strings.Contains(err.Error(), "outside allowed bounds") || strings.Contains(err.Error(), "transaction data") {
		code = continuity.CodeMigrationTooLarge
	}
	if strings.Contains(err.Error(), "secret") {
		return Receipt{Accepted: false, Rejection: &Rejection{Code: "SENSITIVE_CONTENT", Message: err.Error()}}
	}
	return Receipt{Accepted: false, Rejection: &Rejection{Code: string(code), Message: err.Error()}}
}

func validateLegacyMigrationDraft(projectID string, events []ledger.Event) error {
	return ledger.ValidateDraft(ledger.Draft{
		TransactionID: "tx_migration_validation", ProjectID: projectID,
		CommandID: "cmd_migration_validation", CommandDigest: "migration-validation",
		IdempotencyKey: "migration-validation", CorrelationID: "migration-validation",
		IssuedAt: time.Unix(1, 0).UTC(), Actor: json.RawMessage(`{"role":"user"}`), Events: events,
	})
}

func migrationQueryError(err error) error {
	code := continuity.CodeMigrationVerifyFailed
	message := err.Error()
	if strings.Contains(message, "limit") || strings.Contains(message, "exceeds") || strings.Contains(message, "outside allowed bounds") || strings.Contains(message, "transaction data") {
		code = continuity.CodeMigrationTooLarge
	}
	if strings.Contains(message, "secret") {
		code = continuity.CodeMigrationNotApplicable
	}
	return &continuity.Error{Code: code, Op: "preview legacy migration", Err: err}
}
