package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

const Version = "0.1.0-dev"

type options struct {
	command           string
	positionals       []string
	done              []string
	next              []string
	validation        []string
	blockers          []string
	mustRead          []string
	clearBlockers     bool
	replaceDone       bool
	replaceValidation bool
	repo              string
	json              bool
	dryRun            bool
	rollback          bool
	help              bool
	version           bool
}

func Run(ctx context.Context, args []string, cwd string, stdout, stderr io.Writer) int {
	jsonIntent := false
	for _, argument := range args {
		if argument == "--json" {
			jsonIntent = true
			break
		}
	}
	options, err := parseArgs(args)
	if err != nil {
		return writeArgumentError(stdout, stderr, jsonIntent, err)
	}
	if options.version {
		fmt.Fprintf(stdout, "summer %s\n", Version)
		return 0
	}
	if options.help {
		fmt.Fprintln(stdout, usage())
		return 0
	}
	root, err := findRoot(options.repo, cwd)
	if err != nil {
		return writeError(stdout, stderr, options.json, continuity.CodeHandoffNotFound, err)
	}
	kernel, err := workspace.Open(root)
	if err != nil {
		return writeError(stdout, stderr, options.json, continuity.CodeHandoffInvalid, err)
	}
	switch options.command {
	case "start":
		projectID := ""
		expectedRevision := uint64(0)
		if view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume}); queryErr == nil {
			resume, ok := view.(engine.ResumeView)
			if !ok {
				return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
			}
			if resume.Capsule.Schema == continuity.CapsuleSchemaV2 && resume.Capsule.Mode == continuity.ModeNative {
				projectID = resume.Capsule.ProjectID
				expectedRevision = resume.Capsule.LedgerRevision
			} else if resume.Capsule.Mode != continuity.ModeDirect && resume.Capsule.Mode != continuity.ModeIdle {
				code := continuity.CodeMigrationRequired
				if resume.Capsule.Mode == continuity.ModeGSD {
					code = continuity.CodeLifecycleConflict
				}
				return writeError(stdout, stderr, options.json, code, errors.New("start cannot replace the current lifecycle"))
			}
		} else if code := continuity.ErrorCode(queryErr); code != continuity.CodeHandoffNotFound {
			if code == "" {
				return writeInternalError(stdout, stderr, options.json, queryErr)
			}
			return writeError(stdout, stderr, options.json, code, queryErr)
		}
		if projectID == "" {
			var idErr error
			projectID, idErr = newCLIIdentifier("project")
			if idErr != nil {
				return writeInternalError(stdout, stderr, options.json, idErr)
			}
		}
		commandID, idErr := newCLIIdentifier("cmd")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		sessionID, idErr := newCLIIdentifier("session")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		goal := strings.TrimSpace(options.positionals[0])
		payload, marshalErr := json.Marshal(engine.StartObjective{Goal: goal, Next: options.next})
		if marshalErr != nil {
			return writeInternalError(stdout, stderr, options.json, marshalErr)
		}
		receipt, applyErr := kernel.Apply(ctx, engine.CommandEnvelope{
			Schema: engine.CommandSchemaV2, CommandID: commandID, IdempotencyKey: commandID,
			CorrelationID: commandID, ProjectID: projectID, ExpectedRevision: expectedRevision,
			Actor:    engine.ActorRef{ActorID: "user-local", SessionID: sessionID, Runtime: "summer-cli", Role: engine.ActorUser},
			IssuedAt: time.Now().UTC(), Kind: engine.CommandStartObjective, Payload: payload,
		})
		if applyErr != nil {
			return writeInternalError(stdout, stderr, options.json, applyErr)
		}
		if !receipt.Accepted {
			return writeEngineRejection(stdout, stderr, options.json, receipt.Rejection)
		}
		projection := "unavailable"
		if receipt.Projection != nil {
			projection = string(receipt.Projection.Status)
		}
		repairRequired := receipt.Projection != nil && receipt.Projection.Status == engine.ProjectionRepairRequired
		result := map[string]any{
			"schema": "summer.cli-result/v1", "ok": !repairRequired, "committed": true, "command": "start",
			"project_id": projectID, "objective_id": receipt.EntityID, "status": "active",
			"ledger_revision": receipt.NewRevision, "transaction_id": receipt.TransactionID,
			"projection": projection,
		}
		if repairRequired {
			result["code"] = receipt.Projection.Code
			result["warning"] = "canonical transaction 已提交，但 Handoff/Snapshot 投影需要修复"
			result["hint"] = "运行 summer doctor；修复投影后再继续"
		}
		next := options.next
		if len(next) == 0 {
			next = []string{goal}
		}
		if err := writeStartOutput(stdout, options.json, result, goal, next); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		if repairRequired {
			return 3
		}
		return 0
	case "save":
		view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume})
		if queryErr != nil {
			code := continuity.ErrorCode(queryErr)
			if code == "" {
				return writeInternalError(stdout, stderr, options.json, queryErr)
			}
			return writeError(stdout, stderr, options.json, code, queryErr)
		}
		resume, ok := view.(engine.ResumeView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
		}
		if resume.Capsule.Schema != continuity.CapsuleSchemaV2 || resume.Capsule.Mode != continuity.ModeNative {
			return writeError(stdout, stderr, options.json, continuity.CodeMigrationRequired, errors.New("save requires a native v2 lifecycle"))
		}
		commandID, idErr := newCLIIdentifier("cmd")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		sessionID, idErr := newCLIIdentifier("session")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		save := engine.SaveObjective{
			ObjectiveID: resume.Capsule.ObjectiveID, ExpectedObjectiveRevision: resume.Capsule.Revision,
			Done: options.done, ReplaceDone: options.replaceDone,
			Validation: options.validation, ReplaceValidation: options.replaceValidation,
		}
		if len(options.next) > 0 {
			save.Next = &options.next
		}
		if len(options.blockers) > 0 {
			save.Blockers = &options.blockers
		} else if options.clearBlockers {
			empty := []string{}
			save.Blockers = &empty
		}
		if len(options.mustRead) > 0 {
			save.MustRead = &options.mustRead
		}
		payload, marshalErr := json.Marshal(save)
		if marshalErr != nil {
			return writeInternalError(stdout, stderr, options.json, marshalErr)
		}
		receipt, applyErr := kernel.Apply(ctx, engine.CommandEnvelope{
			Schema: engine.CommandSchemaV2, CommandID: commandID, IdempotencyKey: commandID,
			CorrelationID: commandID, ProjectID: resume.Capsule.ProjectID,
			ExpectedRevision: resume.Capsule.LedgerRevision,
			Actor:            engine.ActorRef{ActorID: "user-local", SessionID: sessionID, Runtime: "summer-cli", Role: engine.ActorUser},
			IssuedAt:         time.Now().UTC(), Kind: engine.CommandSaveObjective, Payload: payload,
		})
		if applyErr != nil {
			return writeInternalError(stdout, stderr, options.json, applyErr)
		}
		if !receipt.Accepted {
			return writeEngineRejection(stdout, stderr, options.json, receipt.Rejection)
		}
		projection := "unavailable"
		if receipt.Projection != nil {
			projection = string(receipt.Projection.Status)
		}
		repairRequired := receipt.Projection != nil && receipt.Projection.Status == engine.ProjectionRepairRequired
		result := map[string]any{
			"schema": "summer.cli-result/v1", "ok": !repairRequired, "committed": true, "command": "save",
			"project_id": resume.Capsule.ProjectID, "objective_id": receipt.EntityID,
			"status":             receipt.EntityStatus,
			"objective_revision": receipt.EntityRevision, "ledger_revision": receipt.NewRevision,
			"transaction_id": receipt.TransactionID, "projection": projection,
		}
		if repairRequired {
			result["code"] = receipt.Projection.Code
			result["warning"] = "canonical transaction 已提交，但 Handoff/Snapshot 投影需要修复"
			result["hint"] = "运行 summer doctor；修复投影后再继续"
		}
		if err := writeSaveOutput(stdout, options.json, result); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		if repairRequired {
			return 3
		}
		return 0
	case "resume":
		view, err := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume})
		if err != nil {
			code := continuity.ErrorCode(err)
			if code == "" {
				return writeInternalError(stdout, stderr, options.json, err)
			}
			return writeError(stdout, stderr, options.json, code, err)
		}
		resume, ok := view.(engine.ResumeView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
		}
		if err := writeResumeOutput(stdout, options.json, resume.Capsule); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		return 0
	case "migrate":
		if options.rollback {
			view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryLegacyRollback})
			if queryErr != nil {
				code := continuity.ErrorCode(queryErr)
				if code == "" {
					return writeInternalError(stdout, stderr, options.json, queryErr)
				}
				return writeError(stdout, stderr, options.json, code, queryErr)
			}
			rollbackView, ok := view.(engine.LegacyRollbackView)
			if !ok {
				return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected rollback view %T", view))
			}
			rollback := rollbackView.Rollback
			commandID, idErr := newCLIIdentifier("cmd")
			if idErr != nil {
				return writeInternalError(stdout, stderr, options.json, idErr)
			}
			sessionID, idErr := newCLIIdentifier("session")
			if idErr != nil {
				return writeInternalError(stdout, stderr, options.json, idErr)
			}
			payload, marshalErr := json.Marshal(engine.RollbackLegacyMigration{
				MigrationID: rollback.MigrationID, ExpectedTransactionID: rollback.TransactionID, ExpectedLedgerHead: rollback.LedgerHead,
			})
			if marshalErr != nil {
				return writeInternalError(stdout, stderr, options.json, marshalErr)
			}
			receipt, applyErr := kernel.Apply(ctx, engine.CommandEnvelope{
				Schema: engine.CommandSchemaV2, CommandID: commandID, IdempotencyKey: commandID,
				CorrelationID: commandID, ProjectID: rollback.ProjectID, ExpectedRevision: 1,
				Actor:    engine.ActorRef{ActorID: "user-local", SessionID: sessionID, Runtime: "summer-cli", Role: engine.ActorUser},
				IssuedAt: time.Now().UTC(), Kind: engine.CommandRollbackLegacyMigration, Payload: payload,
			})
			if applyErr != nil {
				return writeInternalError(stdout, stderr, options.json, applyErr)
			}
			if !receipt.Accepted {
				return writeEngineRejection(stdout, stderr, options.json, receipt.Rejection)
			}
			rollback.RolledBack = true
			return writeRollbackOutput(stdout, options.json, rollback)
		}
		view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryLegacyMigration})
		if queryErr != nil {
			code := continuity.ErrorCode(queryErr)
			if code == "" {
				return writeInternalError(stdout, stderr, options.json, queryErr)
			}
			return writeError(stdout, stderr, options.json, code, queryErr)
		}
		migrationView, ok := view.(engine.LegacyMigrationView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected migration view %T", view))
		}
		plan := migrationView.Migration
		if options.dryRun || (migrationView.Committed && !migrationView.SwitchPending) {
			result := migrationResult(plan, migrationView.Committed, migrationView.LedgerRevision, migrationView.TransactionID)
			if err := writeMigrationOutput(stdout, options.json, result, options.dryRun, migrationView.Committed); err != nil {
				return writeInternalError(stdout, stderr, options.json, err)
			}
			return 0
		}
		commandID, idErr := newCLIIdentifier("cmd")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		sessionID, idErr := newCLIIdentifier("session")
		if idErr != nil {
			return writeInternalError(stdout, stderr, options.json, idErr)
		}
		payload, marshalErr := json.Marshal(engine.ImportLegacyNative{
			MigrationID: plan.MigrationID, SourceDigest: plan.SourceDigest, BackupManifestDigest: plan.BackupManifestDigest,
		})
		if marshalErr != nil {
			return writeInternalError(stdout, stderr, options.json, marshalErr)
		}
		receipt, applyErr := kernel.Apply(ctx, engine.CommandEnvelope{
			Schema: engine.CommandSchemaV2, CommandID: commandID, IdempotencyKey: commandID,
			CorrelationID: commandID, ProjectID: plan.ProjectID, ExpectedRevision: 0,
			Actor:    engine.ActorRef{ActorID: "user-local", SessionID: sessionID, Runtime: "summer-cli", Role: engine.ActorUser},
			IssuedAt: time.Now().UTC(), Kind: engine.CommandImportLegacyNative, Payload: payload,
		})
		if applyErr != nil {
			return writeInternalError(stdout, stderr, options.json, applyErr)
		}
		if !receipt.Accepted {
			return writeEngineRejection(stdout, stderr, options.json, receipt.Rejection)
		}
		result := migrationResult(plan, true, receipt.NewRevision, receipt.TransactionID)
		if err := writeMigrationOutput(stdout, options.json, result, false, true); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		return 0
	case "doctor":
		view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume})
		if queryErr != nil {
			code := continuity.ErrorCode(queryErr)
			if code == "" {
				code = "INTERNAL_ERROR"
			}
			result := map[string]any{
				"ok": false, "mode": "unknown", "issues": []map[string]string{{"code": string(code), "message": messageForCode(code)}},
				"warnings": []string{},
			}
			if err := writeDoctorOutput(stdout, options.json, result); err != nil {
				return writeInternalError(stdout, stderr, options.json, err)
			}
			return 2
		}
		resume, ok := view.(engine.ResumeView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
		}
		result := map[string]any{
			"ok": true, "mode": resume.Capsule.Mode, "schema": resume.Capsule.Schema,
			"issues": []map[string]string{}, "warnings": []string{},
		}
		if err := writeDoctorOutput(stdout, options.json, result); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		return 0
	default:
		fmt.Fprintln(stderr, "error: unsupported command")
		return 2
	}
}

func parseArgs(args []string) (options, error) {
	var result options
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			result.json = true
		case argument == "--help" || argument == "-h":
			result.help = true
		case argument == "--version":
			result.version = true
		case argument == "--dry-run":
			result.dryRun = true
		case argument == "--rollback":
			result.rollback = true
		case argument == "--repo":
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" || strings.HasPrefix(args[index], "-") {
				return options{}, errors.New("--repo requires a path")
			}
			result.repo = args[index]
		case strings.HasPrefix(argument, "--repo="):
			result.repo = strings.TrimPrefix(argument, "--repo=")
			if strings.TrimSpace(result.repo) == "" {
				return options{}, errors.New("--repo requires a path")
			}
		case argument == "--clear-blockers":
			result.clearBlockers = true
		case argument == "--replace-done":
			result.replaceDone = true
		case argument == "--replace-validation":
			result.replaceValidation = true
		case argument == "--done" || argument == "--next" || argument == "--validation" || argument == "--blocker" || argument == "--must-read":
			flag := argument
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" || strings.HasPrefix(args[index], "-") {
				return options{}, fmt.Errorf("%s requires a value", flag)
			}
			switch flag {
			case "--done":
				result.done = append(result.done, args[index])
			case "--next":
				result.next = append(result.next, args[index])
			case "--validation":
				result.validation = append(result.validation, args[index])
			case "--blocker":
				result.blockers = append(result.blockers, args[index])
			case "--must-read":
				result.mustRead = append(result.mustRead, args[index])
			}
		case strings.HasPrefix(argument, "-"):
			return options{}, fmt.Errorf("unknown flag %q", argument)
		default:
			if result.command == "" {
				result.command = argument
			} else {
				result.positionals = append(result.positionals, argument)
			}
		}
	}
	if result.version {
		if result.command != "" || len(result.positionals) > 0 {
			return options{}, errors.New("--version does not accept a command")
		}
		return result, nil
	}
	if result.help && result.command == "" {
		return result, nil
	}
	if result.command != "start" && result.command != "save" && result.command != "resume" && result.command != "doctor" && result.command != "migrate" {
		return options{}, errors.New("expected the start, save, resume, migrate, or doctor command")
	}
	if result.command == "start" && len(result.positionals) != 1 {
		return options{}, errors.New("start requires exactly one goal")
	}
	if result.command != "start" && len(result.positionals) != 0 {
		return options{}, fmt.Errorf("%s does not accept positional arguments", result.command)
	}
	checkpointFlags := len(result.done) + len(result.validation) + len(result.blockers) + len(result.mustRead)
	if result.clearBlockers {
		checkpointFlags++
	}
	if result.replaceDone {
		checkpointFlags++
	}
	if result.replaceValidation {
		checkpointFlags++
	}
	if result.command != "save" && checkpointFlags > 0 {
		return options{}, fmt.Errorf("checkpoint flags require the save command")
	}
	if result.command != "start" && result.command != "save" && len(result.next) > 0 {
		return options{}, errors.New("--next requires the start or save command")
	}
	if result.dryRun && result.command != "migrate" {
		return options{}, errors.New("--dry-run requires the migrate command")
	}
	if result.rollback && result.command != "migrate" {
		return options{}, errors.New("--rollback requires the migrate command")
	}
	if result.rollback && result.dryRun {
		return options{}, errors.New("--rollback cannot be combined with --dry-run")
	}
	if result.clearBlockers && len(result.blockers) > 0 {
		return options{}, errors.New("--clear-blockers cannot be combined with --blocker")
	}
	return result, nil
}

func newCLIIdentifier(prefix string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("create %s identifier: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}

func findRoot(explicit, cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(cwd, explicit)
		}
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
			return "", fmt.Errorf("repository path is not a directory: %s", absolute)
		}
		resolved, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", fmt.Errorf("resolve repository path: %w", err)
		}
		return resolved, nil
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		return "", fmt.Errorf("current path is not a directory: %s", current)
	}
	for {
		handoff := filepath.Join(current, ".agent", "HANDOFF.md")
		if info, err := os.Lstat(handoff); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}
		if _, err := os.Lstat(filepath.Join(current, ".agent")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("no .agent/HANDOFF.md found from the current directory")
		}
		current = parent
	}
}

func writeJSON(writer io.Writer, value any, indent bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if indent {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func writeStartOutput(writer io.Writer, jsonOutput bool, result map[string]any, goal string, next []string) error {
	if jsonOutput {
		return writeJSON(writer, result, false)
	}
	if _, err := fmt.Fprintf(writer, "已启动 Summer Harness\n目标: %s\n状态: active\n下一步:\n", goal); err != nil {
		return err
	}
	for _, item := range next {
		if _, err := fmt.Fprintf(writer, "- %s\n", item); err != nil {
			return err
		}
	}
	if result["projection"] == string(engine.ProjectionRepairRequired) {
		_, err := fmt.Fprintf(writer, "警告: %v\n修复: %v\n", result["warning"], result["hint"])
		return err
	}
	return nil
}

func writeSaveOutput(writer io.Writer, jsonOutput bool, result map[string]any) error {
	if jsonOutput {
		return writeJSON(writer, result, false)
	}
	_, err := fmt.Fprintf(writer, "已保存 checkpoint\n状态: %v\n目标修订: %v\n", result["status"], result["objective_revision"])
	if err == nil && result["projection"] == string(engine.ProjectionRepairRequired) {
		_, err = fmt.Fprintf(writer, "警告: %v\n修复: %v\n", result["warning"], result["hint"])
	}
	return err
}

func writeResumeOutput(writer io.Writer, jsonOutput bool, capsule continuity.Capsule) error {
	if jsonOutput {
		return writeJSON(writer, capsule, false)
	}
	status := capsule.Status
	if status == "" {
		status = string(capsule.Mode)
	}
	if _, err := fmt.Fprintf(writer, "目标: %s\n状态: %s\n", capsule.Goal, status); err != nil {
		return err
	}
	for _, group := range []struct {
		label  string
		values []string
	}{{"下一步", capsule.Next}, {"阻塞", capsule.Blockers}, {"验证", capsule.Validation}, {"必须读取", capsule.MustRead}} {
		if len(group.values) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(writer, "%s:\n", group.label); err != nil {
			return err
		}
		for _, value := range group.values {
			if _, err := fmt.Fprintf(writer, "- %s\n", value); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeDoctorOutput(writer io.Writer, jsonOutput bool, result map[string]any) error {
	if jsonOutput {
		return writeJSON(writer, result, false)
	}
	health := "异常"
	if ok, _ := result["ok"].(bool); ok {
		health = "正常"
	}
	_, err := fmt.Fprintf(writer, "健康: %s\n模式: %v\n", health, result["mode"])
	return err
}

func migrationResult(plan continuity.LegacyMigration, committed bool, revision uint64, transactionID string) map[string]any {
	return map[string]any{
		"schema": "summer.migration-result/v1", "ok": true, "command": "migrate",
		"committed": committed, "migration_id": plan.MigrationID, "project_id": plan.ProjectID,
		"source_digest": plan.SourceDigest, "semantic_digest": plan.SemanticDigest,
		"backup_manifest_digest": plan.BackupManifestDigest, "counts": plan.Counts,
		"active_objective_id": plan.ActiveObjectiveID, "ledger_revision": revision, "transaction_id": transactionID,
	}
}

func writeMigrationOutput(writer io.Writer, jsonOutput bool, result map[string]any, dryRun, committed bool) error {
	if jsonOutput {
		return writeJSON(writer, result, false)
	}
	counts, _ := result["counts"].(continuity.LegacyMigrationCounts)
	status := "dry-run 通过，未写入任何文件"
	if committed {
		status = "迁移完成，v2 Canonical Ledger 已生效"
	}
	if !dryRun && !committed {
		status = "迁移尚未提交"
	}
	_, err := fmt.Fprintf(writer, "%s\n目标: %d\nDecision: %d\nFact: %d\nFact invalidation: %d\n", status, counts.Objectives, counts.Decisions, counts.Facts, counts.FactInvalidations)
	return err
}

func writeRollbackOutput(writer io.Writer, jsonOutput bool, rollback continuity.LegacyRollback) int {
	if jsonOutput {
		if err := writeJSON(writer, map[string]any{
			"schema": "summer.migration-result/v1", "ok": true, "command": "migrate --rollback",
			"migration_id": rollback.MigrationID, "project_id": rollback.ProjectID, "rolled_back": true,
		}, false); err != nil {
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintln(writer, "迁移已回滚，原始 v1 Handoff 已恢复"); err != nil {
		return 1
	}
	return 0
}

func writeError(stdout, stderr io.Writer, jsonOutput bool, code continuity.Code, cause error) int {
	message := messageForCode(code)
	hint := hintForCode(code)
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{"ok": false, "code": code, "error": message, "hint": hint}, false)
	} else {
		fmt.Fprintln(stderr, "错误:", message)
		if hint != "" {
			fmt.Fprintln(stderr, "提示:", hint)
		}
	}
	_ = cause
	return 2
}

func writeInternalError(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{"ok": false, "code": "INTERNAL_ERROR", "error": "Summer Harness 内部错误", "hint": "运行 summer doctor 并保留错误输出"}, false)
	} else {
		fmt.Fprintln(stderr, "错误: Summer Harness 内部错误")
		fmt.Fprintln(stderr, "详情:", cause)
	}
	return 1
}

func writeArgumentError(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	message := "参数无效，请检查命令、参数归属和必填值"
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{
			"ok": false, "code": "INVALID_ARGUMENT", "error": message, "hint": shortUsage(),
		}, false)
	} else {
		fmt.Fprintln(stderr, "错误:", message)
		fmt.Fprintln(stderr, shortUsage())
	}
	_ = cause
	return 2
}

func writeEngineRejection(stdout, stderr io.Writer, jsonOutput bool, rejection *engine.Rejection) int {
	if rejection == nil {
		return writeInternalError(stdout, stderr, jsonOutput, errors.New("engine rejected command without a reason"))
	}
	message := localizedEngineMessage(rejection.Code)
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{
			"ok": false, "code": rejection.Code, "error": message, "hint": "运行 summer doctor 获取诊断结果",
		}, false)
	} else {
		fmt.Fprintf(stderr, "错误: %s\n错误码: %s\n", message, rejection.Code)
	}
	return 2
}

func localizedEngineMessage(code string) string {
	switch code {
	case "OBJECTIVE_EXISTS":
		return "项目已有未结束的 Root Objective"
	case "NO_ACTIVE_OBJECTIVE":
		return "项目没有可保存的 Root Objective"
	case "OBJECTIVE_NOT_CURRENT", "OBJECTIVE_REVISION_CONFLICT", "REVISION_CONFLICT":
		return "状态已被其他写入更新，请先重新恢复"
	case "SENSITIVE_CONTENT":
		return "内容疑似包含密钥，已拒绝写入"
	case "INVALID_COMMAND":
		return "命令内容不符合领域约束"
	case "FORBIDDEN":
		return "当前身份无权执行此操作"
	case "PROJECT_CONFLICT":
		return "Canonical Ledger 属于另一个项目"
	case "IDEMPOTENCY_CONFLICT":
		return "幂等键已被其他命令使用"
	case string(continuity.CodeMigrationIncomplete):
		return "迁移未完整收敛，不能宣称完成"
	case string(continuity.CodeMigrationSourceChanged):
		return "迁移期间 v1 权威状态发生变化"
	case string(continuity.CodeRollbackNotAllowed):
		return "当前迁移不满足回滚条件"
	case string(continuity.CodeRollbackSourceDrift):
		return "回滚备份与 Canonical Genesis 不一致"
	default:
		return "命令被 Summer Harness 拒绝"
	}
}

func messageForCode(code continuity.Code) string {
	switch code {
	case continuity.CodeHandoffNotFound:
		return "未找到 .agent/HANDOFF.md"
	case continuity.CodeHandoffTooLarge:
		return "Handoff 超过 4 KiB 限制"
	case continuity.CodeHandoffInvalid:
		return "Handoff 格式或内容无效"
	case continuity.CodeHandoffUnsupportedSchema:
		return "Handoff schema 不受支持"
	case continuity.CodeHandoffDrift:
		return "Handoff 与权威状态不一致"
	case continuity.CodeUnsafeReference:
		return "Handoff 包含不安全的文件引用"
	case continuity.CodeGSDPointerStale:
		return "GSD 状态指针已过期"
	case continuity.CodeCapsuleTooLarge:
		return "恢复胶囊超过 32 KiB 限制"
	case continuity.CodeProjectionStale, continuity.CodeProjectionConflict:
		return "Handoff 投影需要修复"
	case continuity.CodeCapabilityUnavailable:
		return "当前安装不支持该恢复能力"
	case continuity.CodeMigrationRequired:
		return "当前项目存在旧版生命周期状态，需要显式迁移"
	case continuity.CodeMigrationNotApplicable:
		return "当前状态不能迁移到 v2"
	case continuity.CodeMigrationSourceChanged:
		return "迁移期间 v1 权威状态发生变化"
	case continuity.CodeMigrationTooLarge:
		return "v1 状态超过单事务迁移上限"
	case continuity.CodeMigrationIncomplete, continuity.CodeMigrationVerifyFailed:
		return "迁移未通过完整性验证"
	case continuity.CodeRollbackNotAllowed:
		return "当前迁移不能回滚"
	case continuity.CodeRollbackSourceDrift:
		return "v1 备份或原始源文件已发生变化"
	case continuity.CodeLifecycleConflict:
		return "当前项目存在冲突的生命周期所有者"
	default:
		return "Summer Harness 状态错误"
	}
}

func hintForCode(code continuity.Code) string {
	switch code {
	case continuity.CodeHandoffDrift, continuity.CodeProjectionStale, continuity.CodeProjectionConflict:
		return "运行 summer doctor；当前开发版请通过已安装的 $project-handoff 显式重建投影"
	case continuity.CodeHandoffNotFound:
		return "确认项目路径，或先使用 Summer Harness 保存交接"
	case continuity.CodeMigrationRequired:
		return "先运行 summer migrate --dry-run，再运行 summer migrate；不要直接覆盖 Handoff 或 Ledger"
	case continuity.CodeMigrationSourceChanged, continuity.CodeMigrationIncomplete, continuity.CodeMigrationVerifyFailed:
		return "停止写入并运行 summer doctor；确认 v1 源文件未变化后重试或回滚"
	case continuity.CodeRollbackNotAllowed, continuity.CodeRollbackSourceDrift:
		return "回滚只允许在迁移后没有任何新 v2 transaction 时执行；不要手工移动 Ledger 文件"
	case continuity.CodeLifecycleConflict:
		return "运行 summer doctor，确认项目只能有一个生命周期所有者"
	default:
		return "运行 summer doctor 获取诊断结果"
	}
}

func shortUsage() string {
	return "用法: summer [--repo <path>] [--json] <start|save|resume|migrate|doctor>"
}

func usage() string {
	return `Summer Harness

用法:
  summer start <goal> [--next <text>] [--repo <path>] [--json]
  summer save [--done <text>] [--replace-done] [--next <text>] [--validation <text>] [--replace-validation] [--blocker <text>] [--must-read <path>]
  summer [--repo <path>] [--json] resume
  summer resume [--repo <path>] [--json]
  summer migrate --dry-run [--repo <path>] [--json]
  summer migrate [--repo <path>] [--json]
  summer migrate --rollback [--repo <path>] [--json]
  summer [--repo <path>] [--json] doctor
  summer doctor [--repo <path>] [--json]
  summer --version`
}
