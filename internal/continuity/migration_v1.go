package continuity

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxMigrationEvents    = 256
	maxMigrationEventData = 1 << 20
	maxMigrationData      = 16 << 20
	maxMigrationFiles     = 1024
)

type LegacyMigrationCounts struct {
	Objectives        int `json:"objectives"`
	Decisions         int `json:"decisions"`
	Facts             int `json:"facts"`
	FactInvalidations int `json:"fact_invalidations"`
}

type LegacyObjectiveImport struct {
	ObjectiveID   string   `json:"objective_id"`
	LegacyTaskID  string   `json:"legacy_task_id"`
	Title         string   `json:"title"`
	Goal          string   `json:"goal"`
	Acceptance    []string `json:"acceptance"`
	Profile       string   `json:"profile"`
	Risk          string   `json:"risk"`
	Status        string   `json:"status"`
	Revision      uint64   `json:"revision"`
	Done          []string `json:"done"`
	Next          []string `json:"next"`
	Validation    []string `json:"validation"`
	Blockers      []string `json:"blockers"`
	MustRead      []string `json:"must_read"`
	ResidualRisks []string `json:"residual_risks"`
}

type LegacyMigrationRecord struct {
	EventKind    string          `json:"event_kind"`
	EntityID     string          `json:"entity_id"`
	LegacyTaskID string          `json:"legacy_task_id"`
	ObjectiveID  string          `json:"objective_id"`
	SourcePath   string          `json:"source_path"`
	SourceDigest string          `json:"source_digest"`
	SourceLine   int             `json:"source_line,omitempty"`
	Data         json.RawMessage `json:"data"`
}

type LegacySourceFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Bytes  int    `json:"bytes"`
	raw    []byte
}

type LegacyMigration struct {
	MigrationID          string                  `json:"migration_id"`
	ProjectID            string                  `json:"project_id"`
	SourceDigest         string                  `json:"source_digest"`
	SemanticDigest       string                  `json:"semantic_digest"`
	BackupManifestDigest string                  `json:"backup_manifest_digest"`
	HandoffDigest        string                  `json:"handoff_digest"`
	ActiveLegacyTaskID   string                  `json:"active_legacy_task_id"`
	ActiveObjectiveID    string                  `json:"active_objective_id"`
	Counts               LegacyMigrationCounts   `json:"counts"`
	Objectives           []LegacyObjectiveImport `json:"objectives"`
	Records              []LegacyMigrationRecord `json:"records"`
	SourceFiles          []LegacySourceFile      `json:"source_files"`
}

type LegacyRollback struct {
	MigrationID          string `json:"migration_id"`
	ProjectID            string `json:"project_id"`
	TransactionID        string `json:"transaction_id"`
	LedgerHead           string `json:"ledger_head"`
	SourceDigest         string `json:"source_digest"`
	SemanticDigest       string `json:"semantic_digest"`
	BackupManifestDigest string `json:"backup_manifest_digest"`
	HandoffDigest        string `json:"handoff_digest"`
	RolledBack           bool   `json:"rolled_back"`
}

type migrationBackupFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Bytes  int    `json:"bytes"`
}

type migrationBackup struct {
	Schema         string                `json:"schema"`
	MigrationID    string                `json:"migration_id"`
	ProjectID      string                `json:"project_id"`
	SourceDigest   string                `json:"source_digest"`
	SemanticDigest string                `json:"semantic_digest"`
	Files          []migrationBackupFile `json:"files"`
	manifestDigest string
}

func (m *Module) InspectLegacyNative(ctx context.Context) (LegacyMigration, error) {
	if err := ctx.Err(); err != nil {
		return LegacyMigration{}, err
	}
	handoffRaw, err := m.readHandoff()
	if err != nil {
		return LegacyMigration{}, err
	}
	metaRaw, err := splitMarkdown(handoffRaw)
	if err != nil {
		return LegacyMigration{}, wrap(CodeHandoffInvalid, "inspect legacy migration", ".agent/HANDOFF.md", err)
	}
	var handoff legacyHandoff
	if err := decodeStrictJSON(metaRaw, &handoff); err != nil {
		return LegacyMigration{}, wrap(CodeHandoffInvalid, "inspect legacy migration", ".agent/HANDOFF.md", err)
	}
	if err := validateLegacyHandoff(handoff); err != nil {
		return LegacyMigration{}, wrap(CodeHandoffInvalid, "inspect legacy migration", ".agent/HANDOFF.md", err)
	}
	expectedHandoff, err := renderLegacyHandoff(handoff)
	if err != nil || !bytes.Equal(handoffRaw, expectedHandoff) {
		return LegacyMigration{}, wrap(CodeHandoffDrift, "validate legacy handoff body", ".agent/HANDOFF.md", errors.New("handoff frontmatter and body differ"))
	}
	if handoff.Mode != ModeNative {
		return LegacyMigration{}, wrap(CodeMigrationNotApplicable, "inspect legacy migration", ".agent/HANDOFF.md", errors.New("only an active v1 native lifecycle can be imported"))
	}
	plan := LegacyMigration{ActiveLegacyTaskID: handoff.TaskID}
	if err := m.addLegacySourceFile(&plan, ".agent/HANDOFF.md", handoffRaw); err != nil {
		return LegacyMigration{}, err
	}
	if raw, readErr := readRegularFile(filepath.Join(m.root, ".agent", "harness.json"), 256<<10); readErr == nil {
		if err := m.addLegacySourceFile(&plan, ".agent/harness.json", raw); err != nil {
			return LegacyMigration{}, err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return LegacyMigration{}, wrap(CodeHandoffInvalid, "inspect legacy config", ".agent/harness.json", readErr)
	}

	tasks, records, err := m.scanLegacyTasks(&plan)
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.Records = append(plan.Records, records...)
	activeTask, exists := tasks[handoff.TaskID]
	if !exists {
		return LegacyMigration{}, wrap(CodeMigrationVerifyFailed, "inspect legacy migration", handoff.SourcePath, errors.New("active handoff task was not found"))
	}
	expectedSource := filepath.ToSlash(filepath.Join(".agent", "ledger", "tasks", handoff.TaskID+".md"))
	if handoff.SourcePath != expectedSource || sourceDigest(plan.SourceFiles, expectedSource) != handoff.SourceDigest {
		return LegacyMigration{}, wrap(CodeHandoffDrift, "validate legacy handoff source", handoff.SourcePath, errors.New("handoff source does not match the active task"))
	}
	if err := compareLegacyProjection(handoff, activeTask); err != nil {
		return LegacyMigration{}, wrap(CodeHandoffDrift, "validate legacy handoff projection", ".agent/HANDOFF.md", err)
	}
	for _, reference := range activeTask.MustRead {
		if err := m.validateReference(reference, ""); err != nil {
			return LegacyMigration{}, err
		}
	}
	decisions, err := m.scanLegacyDecisionsForMigration(&plan, tasks)
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.Records = append(plan.Records, decisions...)
	facts, err := m.scanLegacyFactsForMigration(&plan, tasks)
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.Records = append(plan.Records, facts...)

	sort.Slice(plan.SourceFiles, func(i, j int) bool { return plan.SourceFiles[i].Path < plan.SourceFiles[j].Path })
	sourceMeta := make([]LegacySourceFile, len(plan.SourceFiles))
	for index, file := range plan.SourceFiles {
		sourceMeta[index] = LegacySourceFile{Path: file.Path, Digest: file.Digest, Bytes: file.Bytes}
	}
	sourceRaw, err := json.Marshal(sourceMeta)
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.SourceDigest = fmt.Sprintf("%x", sha256.Sum256(sourceRaw))
	plan.MigrationID = "migration_" + plan.SourceDigest[:24]
	if err := m.rejectMigrationAfterRollback(plan.MigrationID); err != nil {
		return LegacyMigration{}, err
	}
	plan.ProjectID = "project_" + digestID(plan.SourceDigest+":"+handoff.TaskID)
	for index := range plan.Objectives {
		plan.Objectives[index].ObjectiveID = "obj_" + digestID(plan.ProjectID+":"+plan.Objectives[index].LegacyTaskID)
		if plan.Objectives[index].LegacyTaskID == handoff.TaskID {
			plan.ActiveObjectiveID = plan.Objectives[index].ObjectiveID
		}
	}
	objectiveByTask := make(map[string]string, len(plan.Objectives))
	for _, objective := range plan.Objectives {
		objectiveByTask[objective.LegacyTaskID] = objective.ObjectiveID
	}
	for index := range plan.Records {
		plan.Records[index].ObjectiveID = objectiveByTask[plan.Records[index].LegacyTaskID]
	}
	if plan.ActiveObjectiveID == "" {
		return LegacyMigration{}, wrap(CodeMigrationVerifyFailed, "inspect legacy migration", ".agent/ledger/tasks", errors.New("active handoff task was not found"))
	}
	semanticRaw, err := json.Marshal(struct {
		Objectives []LegacyObjectiveImport `json:"objectives"`
		Records    []LegacyMigrationRecord `json:"records"`
	}{plan.Objectives, plan.Records})
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.SemanticDigest = fmt.Sprintf("%x", sha256.Sum256(semanticRaw))
	manifestRaw, err := migrationBackupManifest(plan)
	if err != nil {
		return LegacyMigration{}, err
	}
	plan.BackupManifestDigest = fmt.Sprintf("%x", sha256.Sum256(manifestRaw))
	if err := validateMigrationCapacity(plan); err != nil {
		return LegacyMigration{}, err
	}
	return plan, nil
}

func (m *Module) scanLegacyTasks(plan *LegacyMigration) (map[string]legacyTask, []LegacyMigrationRecord, error) {
	directory := filepath.Join(m.root, ".agent", "ledger", "tasks")
	if err := m.validateLegacyDirectory(".agent/ledger/tasks"); err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, wrap(CodeHandoffInvalid, "scan legacy tasks", ".agent/ledger/tasks", err)
	}
	if len(entries) > maxMigrationEvents {
		return nil, nil, wrap(CodeMigrationTooLarge, "scan legacy tasks", ".agent/ledger/tasks", errors.New("task directory exceeds entry limit"))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	tasks := make(map[string]legacyTask)
	records := make([]LegacyMigrationRecord, 0, len(entries))
	nonterminal := ""
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, nil, wrap(CodeUnsafeReference, "scan legacy tasks", entry.Name(), errors.New("task entry is not a regular file"))
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil, nil, wrap(CodeHandoffInvalid, "scan legacy tasks", entry.Name(), errors.New("unexpected task directory entry"))
		}
		relative := filepath.ToSlash(filepath.Join(".agent", "ledger", "tasks", entry.Name()))
		raw, err := readRegularFile(filepath.Join(m.root, filepath.FromSlash(relative)), 256<<10)
		if err != nil {
			return nil, nil, wrap(CodeHandoffInvalid, "read legacy task", relative, err)
		}
		metaRaw, err := splitMarkdown(raw)
		if err != nil {
			return nil, nil, wrap(CodeHandoffInvalid, "parse legacy task", relative, err)
		}
		var task legacyTask
		if err := decodeStrictJSON(metaRaw, &task); err != nil {
			return nil, nil, wrap(CodeHandoffInvalid, "decode legacy task", relative, err)
		}
		if task.Schema != legacySchema || task.Kind != "task" || task.Engine != "summer" || !legacyTaskIDPattern.MatchString(task.ID) || task.ID+".md" != entry.Name() || task.Title == "" || task.Goal == "" || len(task.Acceptance) == 0 || task.Revision == 0 {
			return nil, nil, wrap(CodeHandoffInvalid, "validate legacy task", relative, errors.New("task identity or content is incomplete"))
		}
		expected, err := renderLegacyTask(task)
		if err != nil || !bytes.Equal(raw, expected) {
			return nil, nil, wrap(CodeHandoffDrift, "validate legacy task body", relative, errors.New("task frontmatter and body differ"))
		}
		status := task.Status
		if status == "done" {
			status = "completed"
		}
		switch status {
		case "active", "blocked", "review":
			if nonterminal != "" {
				return nil, nil, wrap(CodeLifecycleConflict, "scan legacy tasks", relative, errors.New("multiple nonterminal legacy tasks"))
			}
			nonterminal = task.ID
		case "completed", "cancelled":
		default:
			return nil, nil, wrap(CodeHandoffInvalid, "validate legacy task", relative, fmt.Errorf("unsupported status %q", task.Status))
		}
		if _, exists := tasks[task.ID]; exists {
			return nil, nil, wrap(CodeHandoffInvalid, "scan legacy tasks", relative, errors.New("duplicate task id"))
		}
		tasks[task.ID] = task
		data, err := json.Marshal(task)
		if err != nil {
			return nil, nil, err
		}
		if containsContinuitySecret(string(data)) {
			return nil, nil, wrap(CodeMigrationNotApplicable, "scan legacy tasks", relative, errors.New("canonicalized task contains a high-confidence secret pattern"))
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(raw))
		plan.Objectives = append(plan.Objectives, LegacyObjectiveImport{
			LegacyTaskID: task.ID, Title: task.Title, Goal: task.Goal, Acceptance: nonNil(task.Acceptance),
			Profile: task.Profile, Risk: task.Risk, Status: status, Revision: task.Revision,
			Done: nonNil(task.Done), Next: nonNil(task.Next), Validation: nonNil(task.Validation),
			Blockers: nonNil(task.Blockers), MustRead: nonNil(task.MustRead), ResidualRisks: nonNil(task.ResidualRisks),
		})
		records = append(records, LegacyMigrationRecord{EventKind: "ObjectiveImported", EntityID: task.ID, LegacyTaskID: task.ID, SourcePath: relative, SourceDigest: digest, Data: data})
		if err := m.addLegacySourceFile(plan, relative, raw); err != nil {
			return nil, nil, err
		}
	}
	if nonterminal == "" || nonterminal != plan.ActiveLegacyTaskID {
		return nil, nil, wrap(CodeLifecycleConflict, "scan legacy tasks", ".agent/ledger/tasks", errors.New("handoff does not identify the only nonterminal task"))
	}
	plan.Counts.Objectives = len(plan.Objectives)
	return tasks, records, nil
}

func (m *Module) scanLegacyDecisionsForMigration(plan *LegacyMigration, tasks map[string]legacyTask) ([]LegacyMigrationRecord, error) {
	directory := filepath.Join(m.root, ".agent", "ledger", "decisions")
	if err := m.validateOptionalLegacyDirectory(".agent/ledger/decisions"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LegacyMigrationRecord{}, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []LegacyMigrationRecord{}, nil
	}
	remainingEvents := maxMigrationEvents - len(plan.Records)
	if len(entries) > remainingEvents {
		return nil, wrap(CodeMigrationTooLarge, "scan legacy decisions", ".agent/ledger/decisions", errors.New("decision directory exceeds entry limit"))
	}
	if err != nil {
		return nil, wrap(CodeHandoffInvalid, "scan legacy decisions", ".agent/ledger/decisions", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seen := make(map[string]struct{})
	records := make([]LegacyMigrationRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, wrap(CodeUnsafeReference, "scan legacy decisions", entry.Name(), errors.New("decision entry is not a regular file"))
		}
		if filepath.Ext(entry.Name()) != ".md" {
			return nil, wrap(CodeHandoffInvalid, "scan legacy decisions", entry.Name(), errors.New("unexpected decision directory entry"))
		}
		relative := filepath.ToSlash(filepath.Join(".agent", "ledger", "decisions", entry.Name()))
		raw, err := readRegularFile(filepath.Join(m.root, filepath.FromSlash(relative)), 128<<10)
		if err != nil {
			return nil, err
		}
		metaRaw, err := splitMarkdown(raw)
		if err != nil {
			return nil, wrap(CodeHandoffInvalid, "parse legacy decision", relative, err)
		}
		var decision legacyDecision
		if err := decodeStrictJSON(metaRaw, &decision); err != nil {
			return nil, wrap(CodeHandoffInvalid, "decode legacy decision", relative, err)
		}
		if decision.Schema != legacySchema || decision.Kind != "decision" || decision.ID == "" || decision.ID+".md" != entry.Name() || decision.Title == "" || decision.Question == "" || decision.Chosen == "" || decision.Source == "" {
			return nil, wrap(CodeHandoffInvalid, "validate legacy decision", relative, errors.New("decision is incomplete"))
		}
		if _, ok := tasks[decision.TaskID]; !ok {
			return nil, wrap(CodeHandoffInvalid, "validate legacy decision", relative, errors.New("decision references an unknown task"))
		}
		if _, exists := seen[decision.ID]; exists {
			return nil, wrap(CodeHandoffInvalid, "scan legacy decisions", relative, errors.New("duplicate decision id"))
		}
		seen[decision.ID] = struct{}{}
		expected, err := renderLegacyDecision(decision)
		if err != nil || !bytes.Equal(raw, expected) {
			return nil, wrap(CodeHandoffDrift, "validate legacy decision body", relative, errors.New("decision frontmatter and body differ"))
		}
		data, _ := json.Marshal(decision)
		if containsContinuitySecret(string(data)) {
			return nil, wrap(CodeMigrationNotApplicable, "scan legacy decisions", relative, errors.New("canonicalized decision contains a high-confidence secret pattern"))
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(raw))
		records = append(records, LegacyMigrationRecord{EventKind: "DecisionRecorded", EntityID: decision.ID, LegacyTaskID: decision.TaskID, SourcePath: relative, SourceDigest: digest, Data: data})
		if err := m.addLegacySourceFile(plan, relative, raw); err != nil {
			return nil, err
		}
	}
	plan.Counts.Decisions = len(records)
	return records, nil
}

func (m *Module) scanLegacyFactsForMigration(plan *LegacyMigration, tasks map[string]legacyTask) ([]LegacyMigrationRecord, error) {
	directory := filepath.Join(m.root, ".agent", "ledger", "facts")
	if err := m.validateOptionalLegacyDirectory(".agent/ledger/facts"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LegacyMigrationRecord{}, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []LegacyMigrationRecord{}, nil
	}
	if len(entries) > maxMigrationFiles {
		return nil, wrap(CodeMigrationTooLarge, "scan legacy facts", ".agent/ledger/facts", errors.New("fact directory exceeds entry limit"))
	}
	if err != nil {
		return nil, wrap(CodeHandoffInvalid, "scan legacy facts", ".agent/ledger/facts", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seenIDs := make(map[string]struct{})
	factTasks := make(map[string]string)
	records := make([]LegacyMigrationRecord, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil, wrap(CodeUnsafeReference, "scan legacy facts", entry.Name(), errors.New("fact entry is not a regular file"))
		}
		if filepath.Ext(entry.Name()) != ".jsonl" {
			return nil, wrap(CodeHandoffInvalid, "scan legacy facts", entry.Name(), errors.New("unexpected fact directory entry"))
		}
		taskID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if _, ok := tasks[taskID]; !ok {
			return nil, wrap(CodeHandoffInvalid, "scan legacy facts", entry.Name(), errors.New("fact file references an unknown task"))
		}
		relative := filepath.ToSlash(filepath.Join(".agent", "ledger", "facts", entry.Name()))
		raw, err := readRegularFile(filepath.Join(m.root, filepath.FromSlash(relative)), 8<<20)
		if err != nil {
			return nil, err
		}
		fileDigest := fmt.Sprintf("%x", sha256.Sum256(raw))
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var identity struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(line, &identity); err != nil {
				return nil, wrap(CodeHandoffInvalid, "decode legacy fact", fmt.Sprintf("%s:%d", relative, lineNumber), err)
			}
			var entityID, referencedTask, eventKind string
			var data []byte
			switch identity.Kind {
			case "fact":
				var fact legacyFact
				if err := decodeStrictJSON(line, &fact); err != nil || fact.Schema != legacySchema || fact.ID == "" || fact.TaskID != taskID || fact.Statement == "" || fact.Source == "" || len(fact.Tags) > 8 {
					return nil, wrap(CodeHandoffInvalid, "validate legacy fact", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("fact is incomplete"))
				}
				entityID, referencedTask, eventKind = fact.ID, fact.TaskID, "FactRecorded"
				data, _ = json.Marshal(fact)
				factTasks[fact.ID] = fact.TaskID
				plan.Counts.Facts++
			case "fact_invalidation":
				var invalidation legacyFactInvalidation
				if err := decodeStrictJSON(line, &invalidation); err != nil || invalidation.Schema != legacySchema || invalidation.ID == "" || invalidation.TaskID != taskID || invalidation.Invalidates == "" || invalidation.Reason == "" {
					return nil, wrap(CodeHandoffInvalid, "validate legacy fact invalidation", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("fact invalidation is incomplete"))
				}
				if factTasks[invalidation.Invalidates] != invalidation.TaskID {
					return nil, wrap(CodeHandoffInvalid, "validate legacy fact invalidation", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("invalidation references an unknown or future fact"))
				}
				entityID, referencedTask, eventKind = invalidation.ID, invalidation.TaskID, "FactInvalidated"
				data, _ = json.Marshal(invalidation)
				plan.Counts.FactInvalidations++
			default:
				return nil, wrap(CodeHandoffInvalid, "decode legacy fact", fmt.Sprintf("%s:%d", relative, lineNumber), fmt.Errorf("unsupported kind %q", identity.Kind))
			}
			if len(entityID) > 512 {
				return nil, wrap(CodeMigrationTooLarge, "validate legacy fact", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("fact entity id exceeds ledger limit"))
			}
			if containsContinuitySecret(string(data)) {
				return nil, wrap(CodeMigrationNotApplicable, "scan legacy facts", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("canonicalized fact contains a high-confidence secret pattern"))
			}
			if _, exists := seenIDs[entityID]; exists {
				return nil, wrap(CodeHandoffInvalid, "scan legacy facts", fmt.Sprintf("%s:%d", relative, lineNumber), errors.New("duplicate fact record id"))
			}
			if len(plan.Records)+len(records) >= maxMigrationEvents {
				return nil, wrap(CodeMigrationTooLarge, "scan legacy facts", fmt.Sprintf("%s:%d", relative, lineNumber), fmt.Errorf("migration exceeds %d-event limit", maxMigrationEvents))
			}
			seenIDs[entityID] = struct{}{}
			records = append(records, LegacyMigrationRecord{EventKind: eventKind, EntityID: entityID, LegacyTaskID: referencedTask, SourcePath: relative, SourceDigest: fileDigest, SourceLine: lineNumber, Data: data})
		}
		if err := scanner.Err(); err != nil {
			return nil, wrap(CodeHandoffInvalid, "scan legacy facts", relative, err)
		}
		if err := m.addLegacySourceFile(plan, relative, raw); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (m *Module) addLegacySourceFile(plan *LegacyMigration, relative string, raw []byte) error {
	if containsContinuitySecret(string(raw)) {
		return wrap(CodeMigrationNotApplicable, "inspect legacy migration", relative, errors.New("legacy source contains a high-confidence secret pattern"))
	}
	if len(plan.SourceFiles) >= maxMigrationFiles {
		return wrap(CodeMigrationTooLarge, "inspect legacy migration", relative, fmt.Errorf("migration exceeds %d source files", maxMigrationFiles))
	}
	totalBytes := len(raw)
	for _, file := range plan.SourceFiles {
		totalBytes += file.Bytes
	}
	if totalBytes > maxMigrationData {
		return wrap(CodeMigrationTooLarge, "inspect legacy migration", relative, fmt.Errorf("legacy source exceeds %d-byte import limit", maxMigrationData))
	}
	copyRaw := append([]byte(nil), raw...)
	plan.SourceFiles = append(plan.SourceFiles, LegacySourceFile{Path: relative, Digest: fmt.Sprintf("%x", sha256.Sum256(raw)), Bytes: len(raw), raw: copyRaw})
	if relative == ".agent/HANDOFF.md" {
		plan.HandoffDigest = fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	return nil
}

func validateMigrationCapacity(plan LegacyMigration) error {
	events := len(plan.Records) + 1
	if events > maxMigrationEvents {
		return wrap(CodeMigrationTooLarge, "validate legacy migration", "", fmt.Errorf("migration requires %d events; limit is %d", events, maxMigrationEvents))
	}
	total := 0
	for _, record := range plan.Records {
		if len(record.EntityID) > 512 || len(record.EventKind) > 512 {
			return wrap(CodeMigrationTooLarge, "validate legacy migration", record.SourcePath, errors.New("record identity exceeds the ledger field limit"))
		}
		if len(record.Data) > maxMigrationEventData {
			return wrap(CodeMigrationTooLarge, "validate legacy migration", record.SourcePath, errors.New("record exceeds the event size limit"))
		}
		total += len(record.Data) + len(record.EntityID) + len(record.EventKind) + 2048
	}
	if total > maxMigrationData {
		return wrap(CodeMigrationTooLarge, "validate legacy migration", "", errors.New("migration exceeds the transaction data limit"))
	}
	return nil
}

func (m *Module) BackupLegacy(ctx context.Context, plan LegacyMigration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := m.InspectLegacyNative(ctx)
	if err != nil {
		return err
	}
	if !sameLegacySource(plan, current) {
		return wrap(CodeMigrationSourceChanged, "backup legacy migration", ".agent", errors.New("legacy source changed after dry-run"))
	}
	plan = current
	base := filepath.Join(m.root, ".agent", "archive", "migrations", plan.MigrationID, "v1")
	if err := ensureRegularDirectories(base); err != nil {
		return wrap(CodeUnsafeReference, "backup legacy migration", ".agent/archive", err)
	}
	for _, source := range plan.SourceFiles {
		if !validLegacySourcePath(source.Path) || len(source.raw) != source.Bytes || fmt.Sprintf("%x", sha256.Sum256(source.raw)) != source.Digest {
			return wrap(CodeUnsafeReference, "backup legacy migration", source.Path, errors.New("legacy backup source is invalid"))
		}
		relative := strings.TrimPrefix(source.Path, ".agent/")
		target := filepath.Join(base, filepath.FromSlash(relative))
		if relativeToBase, err := filepath.Rel(base, target); err != nil || !filepath.IsLocal(relativeToBase) {
			return wrap(CodeUnsafeReference, "backup legacy migration", source.Path, errors.New("legacy backup target escapes its archive"))
		}
		if err := ensureRegularDirectories(filepath.Dir(target)); err != nil {
			return wrap(CodeUnsafeReference, "backup legacy migration", source.Path, err)
		}
		if existing, err := readRegularFile(target, int64(len(source.raw))+1); err == nil {
			if !bytes.Equal(existing, source.raw) {
				return wrap(CodeMigrationSourceChanged, "backup legacy migration", source.Path, errors.New("existing backup differs"))
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeAtomicFile(target, source.raw, 0o600); err != nil {
			return wrap(CodeProjectionConflict, "backup legacy migration", source.Path, err)
		}
	}
	manifestRaw, err := migrationBackupManifest(plan)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(base, "manifest.json")
	if existing, err := readRegularFile(manifestPath, 1<<20); err == nil {
		if !bytes.Equal(existing, manifestRaw) {
			return wrap(CodeMigrationSourceChanged, "backup legacy migration", manifestPath, errors.New("existing backup manifest differs"))
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeAtomicFile(manifestPath, manifestRaw, 0o600); err != nil {
		return wrap(CodeProjectionConflict, "backup legacy migration", manifestPath, err)
	}
	return nil
}

func (m *Module) VerifyLegacySource(ctx context.Context, expected LegacyMigration) error {
	current, err := m.InspectLegacyNative(ctx)
	if err != nil {
		return err
	}
	if !sameLegacySource(expected, current) {
		return wrap(CodeMigrationSourceChanged, "verify legacy migration source", ".agent", errors.New("legacy source changed before canonical commit"))
	}
	return nil
}

func (m *Module) LegacySwitchPending(ctx context.Context, expectedHandoffDigest string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	raw, err := m.readHandoff()
	if err != nil {
		return false, err
	}
	metaRaw, err := splitMarkdown(raw)
	if err != nil {
		return false, err
	}
	schema, err := readSchema(metaRaw)
	if err != nil {
		return false, err
	}
	switch schema {
	case legacySchema:
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != expectedHandoffDigest {
			return false, wrap(CodeMigrationSourceChanged, "inspect migration switch", ".agent/HANDOFF.md", errors.New("legacy handoff differs from the imported source"))
		}
		return true, nil
	case HandoffSchemaV2:
		return false, nil
	default:
		return false, wrap(CodeLifecycleConflict, "inspect migration switch", ".agent/HANDOFF.md", fmt.Errorf("unsupported handoff schema %q", schema))
	}
}

func (m *Module) InspectLegacyRollback(ctx context.Context) (LegacyRollback, error) {
	if err := ctx.Err(); err != nil {
		return LegacyRollback{}, err
	}
	handoffRaw, err := m.readHandoff()
	if err != nil {
		return LegacyRollback{}, err
	}
	metaRaw, err := splitMarkdown(handoffRaw)
	if err != nil {
		return LegacyRollback{}, wrap(CodeHandoffInvalid, "inspect rollback handoff", ".agent/HANDOFF.md", err)
	}
	schema, err := readSchema(metaRaw)
	if err != nil {
		return LegacyRollback{}, err
	}
	projectID := ""
	rolledBack := false
	if schema == HandoffSchemaV2 {
		var document documentV2
		if err := decodeStrictJSON(metaRaw, &document); err != nil || validateDocumentV2(document) != nil {
			return LegacyRollback{}, wrap(CodeHandoffInvalid, "inspect rollback handoff", ".agent/HANDOFF.md", errors.New("v2 handoff is invalid"))
		}
		projectID = document.ProjectID
	} else if schema == legacySchema {
		rolledBack = true
	} else {
		return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback handoff", ".agent/HANDOFF.md", errors.New("handoff schema is not rollback-compatible"))
	}

	archiveRoot := filepath.Join(m.root, ".agent", "archive", "migrations")
	if err := m.validateOptionalLegacyDirectory(".agent/archive/migrations"); err != nil {
		return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback archive", ".agent/archive/migrations", err)
	}
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		return LegacyRollback{}, err
	}
	if len(entries) > 1000 {
		return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback archive", ".agent/archive/migrations", errors.New("migration archive exceeds entry limit"))
	}
	var selected migrationBackup
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !legacyMigrationID(entry.Name()) {
			return LegacyRollback{}, wrap(CodeUnsafeReference, "inspect rollback archive", entry.Name(), errors.New("migration archive entry is unsafe"))
		}
		backup, loadErr := m.loadMigrationBackup(entry.Name())
		if loadErr != nil {
			if ErrorCode(loadErr) == CodeRollbackNotAllowed {
				continue
			}
			return LegacyRollback{}, loadErr
		}
		if projectID != "" && backup.ProjectID != projectID {
			continue
		}
		if rolledBack {
			original, originalErr := m.backupFileRaw(backup, ".agent/HANDOFF.md")
			if originalErr != nil || !bytes.Equal(original, handoffRaw) {
				continue
			}
			projectID = backup.ProjectID
		}
		if selected.MigrationID != "" {
			return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback archive", ".agent/archive/migrations", errors.New("multiple migration backups match the current project"))
		}
		selected = backup
	}
	if selected.MigrationID == "" {
		return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback archive", ".agent/archive/migrations", errors.New("no migration backup matches the current project"))
	}

	headPaths := []string{
		filepath.Join(m.root, ".agent", "ledger", "HEAD"),
		filepath.Join(m.root, ".agent", "archive", "migrations", selected.MigrationID, "rollback", "v2", "HEAD"),
	}
	var head struct {
		Schema        string `json:"schema"`
		ProjectID     string `json:"project_id"`
		TransactionID string `json:"transaction_id"`
		Revision      uint64 `json:"revision"`
		Digest        string `json:"digest"`
		ResumeDigest  string `json:"resume_digest,omitempty"`
	}
	foundHead := false
	for _, path := range headPaths {
		raw, readErr := readRegularFile(path, 64<<10)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return LegacyRollback{}, readErr
		}
		if err := decodeStrictJSON(raw, &head); err != nil {
			return LegacyRollback{}, err
		}
		foundHead = true
		break
	}
	if !foundHead || head.ProjectID != selected.ProjectID || head.Revision != 1 || head.TransactionID == "" || head.Digest == "" {
		return LegacyRollback{}, wrap(CodeRollbackNotAllowed, "inspect rollback genesis", ".agent/ledger/HEAD", errors.New("migration genesis HEAD is missing or changed"))
	}
	originalHandoff, err := m.backupFileRaw(selected, ".agent/HANDOFF.md")
	if err != nil {
		return LegacyRollback{}, err
	}
	return LegacyRollback{
		MigrationID: selected.MigrationID, ProjectID: selected.ProjectID, TransactionID: head.TransactionID, LedgerHead: head.Digest,
		SourceDigest: selected.SourceDigest, SemanticDigest: selected.SemanticDigest, BackupManifestDigest: selected.manifestDigest,
		HandoffDigest: fmt.Sprintf("%x", sha256.Sum256(originalHandoff)), RolledBack: rolledBack,
	}, nil
}

func (m *Module) PreflightLegacyRollback(ctx context.Context, rollback LegacyRollback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	backup, err := m.loadMigrationBackup(rollback.MigrationID)
	if err != nil {
		return err
	}
	if backup.ProjectID != rollback.ProjectID || backup.SourceDigest != rollback.SourceDigest || backup.SemanticDigest != rollback.SemanticDigest || backup.manifestDigest != rollback.BackupManifestDigest {
		return wrap(CodeRollbackSourceDrift, "preflight legacy rollback", ".agent/archive", errors.New("backup manifest does not match the canonical migration"))
	}
	originalHandoff, err := m.backupFileRaw(backup, ".agent/HANDOFF.md")
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(originalHandoff)) != rollback.HandoffDigest {
		return wrap(CodeRollbackSourceDrift, "preflight legacy rollback", ".agent/HANDOFF.md", errors.New("backup handoff does not match the canonical migration"))
	}
	if err := m.validateRollbackSnapshot(); err != nil {
		return err
	}
	for _, file := range backup.Files {
		raw, err := m.backupFileRaw(backup, file.Path)
		if err != nil {
			return err
		}
		if file.Path == ".agent/HANDOFF.md" {
			continue
		}
		live, readErr := readRegularFile(filepath.Join(m.root, filepath.FromSlash(file.Path)), int64(file.Bytes)+1)
		if readErr != nil || !bytes.Equal(live, raw) {
			return wrap(CodeRollbackSourceDrift, "preflight legacy rollback", file.Path, errors.New("legacy source changed after migration"))
		}
	}
	return nil
}

func (m *Module) RestoreLegacyBackup(ctx context.Context, rollback LegacyRollback) error {
	if err := m.PreflightLegacyRollback(ctx, rollback); err != nil {
		return err
	}
	backup, err := m.loadMigrationBackup(rollback.MigrationID)
	if err != nil {
		return err
	}
	original, err := m.backupFileRaw(backup, ".agent/HANDOFF.md")
	if err != nil {
		return err
	}
	if err := m.validateRollbackSnapshot(); err != nil {
		return err
	}
	current, err := m.readHandoff()
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		metaRaw, splitErr := splitMarkdown(current)
		if splitErr != nil {
			return wrap(CodeRollbackSourceDrift, "restore legacy handoff", ".agent/HANDOFF.md", splitErr)
		}
		var document documentV2
		if decodeErr := decodeStrictJSON(metaRaw, &document); decodeErr != nil || document.Schema != HandoffSchemaV2 || document.ProjectID != rollback.ProjectID {
			return wrap(CodeRollbackSourceDrift, "restore legacy handoff", ".agent/HANDOFF.md", errors.New("current handoff is neither the migrated projection nor the original backup"))
		}
		if err := writeAtomicFile(m.handoffPath, original, 0o644); err != nil {
			return wrap(CodeProjectionConflict, "restore legacy handoff", ".agent/HANDOFF.md", err)
		}
	}
	snapshot := filepath.Join(m.root, ".agent", "cache", "resume.snapshot.json")
	if info, statErr := os.Lstat(snapshot); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return wrap(CodeUnsafeReference, "remove rolled back snapshot", ".agent/cache/resume.snapshot.json", errors.New("snapshot is not a regular file"))
		}
		if err := os.Remove(snapshot); err != nil {
			return err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	return nil
}

func (m *Module) validateRollbackSnapshot() error {
	cache := filepath.Join(m.root, ".agent", "cache")
	if info, err := os.Lstat(cache); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return wrap(CodeUnsafeReference, "inspect rolled back snapshot", ".agent/cache", errors.New("cache is not a regular directory"))
		}
	} else if errors.Is(err, os.ErrNotExist) {
		return nil
	} else {
		return err
	}
	snapshot := filepath.Join(cache, "resume.snapshot.json")
	if info, err := os.Lstat(snapshot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return wrap(CodeUnsafeReference, "inspect rolled back snapshot", ".agent/cache/resume.snapshot.json", errors.New("snapshot is not a regular file"))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m *Module) loadMigrationBackup(migrationID string) (migrationBackup, error) {
	if !legacyMigrationID(migrationID) {
		return migrationBackup{}, wrap(CodeUnsafeReference, "load migration backup", migrationID, errors.New("migration id is invalid"))
	}
	backupDirectory := filepath.ToSlash(filepath.Join(".agent", "archive", "migrations", migrationID, "v1"))
	if err := m.validateLegacyDirectory(backupDirectory); err != nil {
		return migrationBackup{}, wrap(CodeUnsafeReference, "load migration backup", backupDirectory, err)
	}
	path := filepath.Join(m.root, filepath.FromSlash(backupDirectory), "manifest.json")
	raw, err := readRegularFile(path, 1<<20)
	if err != nil {
		return migrationBackup{}, wrap(CodeRollbackNotAllowed, "load migration backup", path, err)
	}
	var backup migrationBackup
	if err := decodeStrictJSON(raw, &backup); err != nil {
		return migrationBackup{}, wrap(CodeRollbackNotAllowed, "load migration backup", path, err)
	}
	if backup.Schema != "summer.migration-backup/v1" || backup.MigrationID != migrationID || backup.ProjectID == "" || backup.SourceDigest == "" || backup.SemanticDigest == "" || len(backup.Files) == 0 {
		return migrationBackup{}, wrap(CodeRollbackNotAllowed, "load migration backup", path, errors.New("migration backup manifest is incomplete"))
	}
	backup.manifestDigest = fmt.Sprintf("%x", sha256.Sum256(raw))
	return backup, nil
}

func (m *Module) backupFileRaw(backup migrationBackup, sourcePath string) ([]byte, error) {
	for _, file := range backup.Files {
		if file.Path != sourcePath {
			continue
		}
		if !validLegacySourcePath(file.Path) || file.Bytes < 0 || len(file.Digest) != sha256.Size*2 {
			return nil, wrap(CodeUnsafeReference, "read migration backup", file.Path, errors.New("backup file identity is invalid"))
		}
		relative := strings.TrimPrefix(file.Path, ".agent/")
		path := filepath.Join(m.root, ".agent", "archive", "migrations", backup.MigrationID, "v1", filepath.FromSlash(relative))
		raw, err := readRegularFile(path, int64(file.Bytes)+1)
		if err != nil || len(raw) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(raw)) != file.Digest {
			return nil, wrap(CodeRollbackSourceDrift, "read migration backup", file.Path, errors.New("backup file differs from its manifest"))
		}
		return raw, nil
	}
	return nil, wrap(CodeRollbackSourceDrift, "read migration backup", sourcePath, errors.New("backup manifest does not contain the required file"))
}

func legacyMigrationID(value string) bool {
	return strings.HasPrefix(value, "migration_") && len(value) <= 128 && legacyTaskIDPattern.MatchString("task_"+strings.TrimPrefix(value, "migration_"))
}

func (m *Module) ProjectMigrated(ctx context.Context, state State, cursor Cursor, expectedLegacyHandoffDigest string) (PublishResult, error) {
	if err := m.PreflightStart(ctx, state, cursor); err != nil {
		return PublishResult{}, err
	}
	raw, err := m.readHandoff()
	if err != nil {
		return PublishResult{}, err
	}
	metaRaw, err := splitMarkdown(raw)
	if err != nil {
		return PublishResult{}, err
	}
	schema, err := readSchema(metaRaw)
	if err != nil {
		return PublishResult{}, err
	}
	if schema == HandoffSchemaV2 {
		return m.Project(ctx, state, cursor)
	}
	if schema != legacySchema || fmt.Sprintf("%x", sha256.Sum256(raw)) != expectedLegacyHandoffDigest {
		return PublishResult{}, wrap(CodeMigrationSourceChanged, "switch migrated handoff", ".agent/HANDOFF.md", errors.New("legacy handoff changed after import"))
	}
	_, projected, err := buildDocumentV2(state, cursor)
	if err != nil {
		return PublishResult{}, err
	}
	if err := m.ensureProjectionDirectories(); err != nil {
		return PublishResult{}, err
	}
	unlock, err := acquireProjectionLock(ctx, filepath.Join(m.root, ".agent", "runtime", "handoff.write.lock"))
	if err != nil {
		return PublishResult{}, wrap(CodeProjectionConflict, "lock migrated handoff", ".agent/runtime/handoff.write.lock", err)
	}
	defer unlock()
	current, err := m.readHandoff()
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(current)) != expectedLegacyHandoffDigest {
		return PublishResult{}, wrap(CodeMigrationSourceChanged, "switch migrated handoff", ".agent/HANDOFF.md", errors.New("legacy handoff changed during switch"))
	}
	if err := writeAtomicFile(m.handoffPath, projected, 0o644); err != nil {
		return PublishResult{}, wrap(CodeProjectionConflict, "switch migrated handoff", ".agent/HANDOFF.md", err)
	}
	if err := m.projectSnapshot(ctx, state, cursor); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{Status: PublishCurrent, Cursor: cursor}, nil
}

func migrationBackupManifest(plan LegacyMigration) ([]byte, error) {
	files := make([]LegacySourceFile, len(plan.SourceFiles))
	for index, file := range plan.SourceFiles {
		files[index] = LegacySourceFile{Path: file.Path, Digest: file.Digest, Bytes: file.Bytes}
	}
	value := struct {
		Schema         string             `json:"schema"`
		MigrationID    string             `json:"migration_id"`
		ProjectID      string             `json:"project_id"`
		SourceDigest   string             `json:"source_digest"`
		SemanticDigest string             `json:"semantic_digest"`
		Files          []LegacySourceFile `json:"files"`
	}{"summer.migration-backup/v1", plan.MigrationID, plan.ProjectID, plan.SourceDigest, plan.SemanticDigest, files}
	raw, err := json.MarshalIndent(value, "", "  ")
	return append(raw, '\n'), err
}

func ensureRegularDirectories(path string) error {
	missing := make([]string, 0)
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("path is not a regular directory: %s", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := os.Mkdir(missing[index], 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(missing[index])
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if err != nil {
				return err
			}
			return fmt.Errorf("path is not a regular directory: %s", missing[index])
		}
		if err := fsyncProjectionDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) rejectMigrationAfterRollback(migrationID string) error {
	tombstonePath := filepath.Join(m.root, ".agent", "archive", "migrations", migrationID, "rollback", "started.json")
	raw, err := readRegularFile(tombstonePath, 64<<10)
	if err == nil {
		var tombstone struct {
			Schema      string `json:"schema"`
			MigrationID string `json:"migration_id"`
			Genesis     struct {
				ProjectID     string `json:"project_id"`
				TransactionID string `json:"transaction_id"`
				Digest        string `json:"digest"`
			} `json:"genesis"`
		}
		if decodeErr := decodeStrictJSON(raw, &tombstone); decodeErr != nil || tombstone.Schema != "summer.migration-rollback-started/v1" || tombstone.MigrationID != migrationID || tombstone.Genesis.ProjectID == "" || tombstone.Genesis.TransactionID == "" || tombstone.Genesis.Digest == "" {
			return wrap(CodeMigrationNotApplicable, "inspect prior rollback", filepath.ToSlash(strings.TrimPrefix(tombstonePath, m.root+string(filepath.Separator))), errors.New("rollback tombstone is invalid"))
		}
		return wrap(CodeMigrationNotApplicable, "inspect prior rollback", filepath.ToSlash(strings.TrimPrefix(tombstonePath, m.root+string(filepath.Separator))), fmt.Errorf("rollback %s has started; re-migration requires an explicit future reset workflow", migrationID))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return wrap(CodeMigrationNotApplicable, "inspect prior rollback", filepath.ToSlash(strings.TrimPrefix(tombstonePath, m.root+string(filepath.Separator))), err)
	}
	path := filepath.Join(m.root, ".agent", "runtime", "migration.rollback.json")
	raw, err = readRegularFile(path, 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return wrap(CodeMigrationNotApplicable, "inspect prior rollback", ".agent/runtime/migration.rollback.json", err)
	}
	var journal struct {
		Schema      string `json:"schema"`
		MigrationID string `json:"migration_id"`
		Stage       string `json:"stage"`
	}
	if err := decodeStrictJSON(raw, &journal); err != nil || journal.Schema != "summer.migration-rollback/v1" || journal.MigrationID == "" {
		return wrap(CodeMigrationNotApplicable, "inspect prior rollback", ".agent/runtime/migration.rollback.json", errors.New("rollback journal is invalid"))
	}
	return wrap(CodeMigrationNotApplicable, "inspect prior rollback", ".agent/runtime/migration.rollback.json", fmt.Errorf("rollback %s is %s; re-migration requires an explicit future reset workflow (requested %s)", journal.MigrationID, journal.Stage, migrationID))
}

func digestID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:12])
}

func sourceDigest(files []LegacySourceFile, path string) string {
	for _, file := range files {
		if file.Path == path {
			return file.Digest
		}
	}
	return ""
}

func (m *Module) validateLegacyDirectory(relative string) error {
	current := m.root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return wrap(CodeHandoffInvalid, "inspect legacy directory", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return wrap(CodeUnsafeReference, "inspect legacy directory", relative, errors.New("legacy directory passes through a symlink or non-directory"))
		}
	}
	return nil
}

func (m *Module) validateOptionalLegacyDirectory(relative string) error {
	path := filepath.Join(m.root, filepath.FromSlash(relative))
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	return m.validateLegacyDirectory(relative)
}

func validLegacySourcePath(path string) bool {
	return strings.HasPrefix(path, ".agent/") && filepath.IsLocal(path) && filepath.Clean(path) == path && !filepath.IsAbs(path)
}

func sameLegacySource(left, right LegacyMigration) bool {
	if left.MigrationID != right.MigrationID || left.ProjectID != right.ProjectID || left.SourceDigest != right.SourceDigest || left.SemanticDigest != right.SemanticDigest || left.BackupManifestDigest != right.BackupManifestDigest || left.HandoffDigest != right.HandoffDigest || len(left.SourceFiles) != len(right.SourceFiles) {
		return false
	}
	for index := range left.SourceFiles {
		a, b := left.SourceFiles[index], right.SourceFiles[index]
		if a.Path != b.Path || a.Digest != b.Digest || a.Bytes != b.Bytes || !bytes.Equal(a.raw, b.raw) {
			return false
		}
	}
	return true
}
