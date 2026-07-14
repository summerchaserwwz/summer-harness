package continuity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type documentV2 struct {
	Schema            string    `json:"schema"`
	Mode              Mode      `json:"mode"`
	Engine            string    `json:"engine"`
	ProjectID         string    `json:"project_id"`
	ObjectiveID       string    `json:"objective_id"`
	ObjectiveStatus   string    `json:"objective_status"`
	ObjectiveRevision uint64    `json:"objective_revision"`
	LedgerRevision    uint64    `json:"ledger_revision"`
	LedgerHead        string    `json:"ledger_head"`
	ProjectorVersion  int       `json:"projector_version"`
	BuiltAt           time.Time `json:"built_at"`
	Goal              string    `json:"goal"`
	Profile           string    `json:"profile"`
	AcceptanceCount   int       `json:"acceptance_count"`
	AcceptanceDigest  string    `json:"acceptance_digest"`
	Done              []string  `json:"done"`
	Next              []string  `json:"next"`
	Validation        []string  `json:"validation"`
	Blockers          []string  `json:"blockers"`
	MustRead          []string  `json:"must_read"`
	ResumeCommand     string    `json:"resume_command"`
	ContentDigest     string    `json:"content_digest"`
}

var continuitySecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{20,}\b`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
}

func (m *Module) Project(ctx context.Context, state State, cursor Cursor) (PublishResult, error) {
	if err := ctx.Err(); err != nil {
		return PublishResult{}, err
	}
	for _, reference := range state.MustRead {
		if err := m.validateReference(reference, ""); err != nil {
			return PublishResult{}, err
		}
	}
	document, raw, err := buildDocumentV2(state, cursor)
	if err != nil {
		return PublishResult{}, wrap(CodeHandoffInvalid, "build handoff projection", ".agent/HANDOFF.md", err)
	}
	if len(raw) > HandoffLimit {
		return PublishResult{}, wrap(CodeHandoffTooLarge, "build handoff projection", ".agent/HANDOFF.md", fmt.Errorf("handoff exceeds %d bytes", HandoffLimit))
	}
	if err := m.ensureProjectionDirectories(); err != nil {
		return PublishResult{}, err
	}
	unlock, err := acquireProjectionLock(ctx, filepath.Join(m.root, ".agent", "runtime", "handoff.write.lock"))
	if err != nil {
		return PublishResult{}, wrap(CodeProjectionConflict, "lock handoff projection", ".agent/runtime/handoff.write.lock", err)
	}
	defer unlock()

	if existing, err := m.readHandoff(); err == nil {
		metaRaw, splitErr := splitMarkdown(existing)
		if splitErr != nil {
			return PublishResult{}, wrap(CodeProjectionConflict, "inspect existing handoff", ".agent/HANDOFF.md", splitErr)
		}
		var identity map[string]json.RawMessage
		if decodeErr := decodeStrictJSON(metaRaw, &identity); decodeErr != nil {
			return PublishResult{}, wrap(CodeProjectionConflict, "inspect existing handoff", ".agent/HANDOFF.md", decodeErr)
		}
		var schema string
		if schemaRaw, ok := identity["schema"]; ok {
			_ = json.Unmarshal(schemaRaw, &schema)
		}
		if schema != HandoffSchemaV2 {
			return PublishResult{}, wrap(CodeProjectionConflict, "publish handoff", ".agent/HANDOFF.md", fmt.Errorf("existing schema %q is not replaceable by v2", schema))
		}
		var current documentV2
		if decodeErr := decodeStrictJSON(metaRaw, &current); decodeErr != nil {
			return PublishResult{}, wrap(CodeProjectionConflict, "inspect existing handoff", ".agent/HANDOFF.md", decodeErr)
		}
		if current.ProjectID != document.ProjectID {
			return PublishResult{}, wrap(CodeProjectionConflict, "publish handoff", ".agent/HANDOFF.md", errors.New("existing handoff belongs to another project"))
		}
		if current.LedgerRevision > document.LedgerRevision {
			return PublishResult{Status: PublishSkipped, Cursor: Cursor{Revision: current.LedgerRevision, Digest: current.LedgerHead}}, wrap(CodeProjectionStale, "publish handoff", ".agent/HANDOFF.md", errors.New("newer projection already exists"))
		}
		if current.LedgerRevision == document.LedgerRevision {
			if current.LedgerHead != document.LedgerHead {
				return PublishResult{}, wrap(CodeProjectionConflict, "publish handoff", ".agent/HANDOFF.md", errors.New("same revision has a different ledger head"))
			}
			if bytes.Equal(existing, raw) {
				return PublishResult{Status: PublishCurrent, Cursor: cursor}, nil
			}
			return PublishResult{}, wrap(CodeProjectionConflict, "publish handoff", ".agent/HANDOFF.md", errors.New("same cursor has different projection content"))
		}
	} else if ErrorCode(err) != CodeHandoffNotFound {
		return PublishResult{}, err
	}

	if err := writeAtomicFile(m.handoffPath, raw, 0o644); err != nil {
		return PublishResult{}, wrap(CodeProjectionConflict, "publish handoff", ".agent/HANDOFF.md", err)
	}
	return PublishResult{Status: PublishCurrent, Cursor: cursor}, nil
}

// PreflightLifecycle prevents a command from creating a second lifecycle
// owner before the canonical ledger is even opened.
func (m *Module) PreflightLifecycle(ctx context.Context, projectID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := m.readHandoff()
	if err == nil {
		metaRaw, splitErr := splitMarkdown(raw)
		if splitErr != nil {
			return wrap(CodeHandoffInvalid, "preflight existing handoff", ".agent/HANDOFF.md", splitErr)
		}
		schema, schemaErr := readSchema(metaRaw)
		if schemaErr != nil {
			return wrap(CodeHandoffInvalid, "preflight existing handoff", ".agent/HANDOFF.md", schemaErr)
		}
		if schema == legacySchema {
			return wrap(CodeMigrationRequired, "preflight lifecycle", ".agent/HANDOFF.md", errors.New("legacy handoff must be migrated explicitly"))
		}
		if schema != HandoffSchemaV2 {
			return wrap(CodeHandoffUnsupportedSchema, "preflight lifecycle", ".agent/HANDOFF.md", fmt.Errorf("unsupported schema %q", schema))
		}
		var current documentV2
		if decodeErr := decodeStrictJSON(metaRaw, &current); decodeErr != nil {
			return wrap(CodeHandoffInvalid, "preflight existing handoff", ".agent/HANDOFF.md", decodeErr)
		}
		if validateErr := validateDocumentV2(current); validateErr != nil {
			return wrap(CodeHandoffInvalid, "preflight existing handoff", ".agent/HANDOFF.md", validateErr)
		}
		if current.ProjectID != projectID {
			return wrap(CodeLifecycleConflict, "preflight lifecycle", ".agent/HANDOFF.md", errors.New("handoff belongs to another project"))
		}
		return wrap(CodeLifecycleConflict, "preflight lifecycle", ".agent/HANDOFF.md", errors.New("workspace already has a v2 lifecycle"))
	} else if ErrorCode(err) != CodeHandoffNotFound {
		return err
	}
	return nil
}

// PreflightStart proves that the first canonical state has bounded, safe
// continuity projections before the transaction is committed.
func (m *Module) PreflightStart(ctx context.Context, state State, cursor Cursor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, reference := range state.MustRead {
		if err := m.validateReference(reference, ""); err != nil {
			return err
		}
	}
	_, raw, err := buildDocumentV2(state, cursor)
	if err != nil {
		return wrap(CodeHandoffInvalid, "preflight handoff projection", ".agent/HANDOFF.md", err)
	}
	if len(raw) > HandoffLimit {
		return wrap(CodeHandoffTooLarge, "preflight handoff projection", ".agent/HANDOFF.md", fmt.Errorf("handoff exceeds %d bytes", HandoffLimit))
	}
	capsule := capsuleV2(state, cursor)
	return checkCapsuleSize(capsule)
}

func (m *Module) resumeV2(ctx context.Context, raw, metaRaw []byte, source Source) (Capsule, error) {
	if source == nil {
		return Capsule{}, wrap(CodeCapabilityUnavailable, "resume v2 handoff", ".agent/HANDOFF.md", errors.New("canonical source is unavailable"))
	}
	var document documentV2
	if err := decodeStrictJSON(metaRaw, &document); err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "decode v2 handoff", ".agent/HANDOFF.md", err)
	}
	if err := validateDocumentV2(document); err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "validate v2 handoff", ".agent/HANDOFF.md", err)
	}
	state, cursor, err := source.Snapshot(ctx, document.ProjectID)
	if err != nil {
		return Capsule{}, err
	}
	_, expected, err := buildDocumentV2(state, cursor)
	if err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "render canonical handoff", ".agent/HANDOFF.md", err)
	}
	if !bytes.Equal(raw, expected) {
		return Capsule{}, wrap(CodeHandoffDrift, "validate v2 handoff", ".agent/HANDOFF.md", errors.New("handoff differs from canonical ledger state"))
	}
	current, err := source.Head(ctx, document.ProjectID)
	if err != nil {
		return Capsule{}, err
	}
	if current != cursor {
		return Capsule{}, wrap(CodeHandoffDrift, "validate v2 snapshot", ".agent/HANDOFF.md", errors.New("ledger changed during resume"))
	}
	for _, reference := range document.MustRead {
		if err := m.validateReference(reference, ""); err != nil {
			return Capsule{}, err
		}
	}
	capsule := capsuleV2(state, cursor)
	if err := checkCapsuleSize(capsule); err != nil {
		return Capsule{}, err
	}
	return capsule, nil
}

func buildDocumentV2(state State, cursor Cursor) (documentV2, []byte, error) {
	acceptance := nonNil(cleanValues(state.Acceptance))
	acceptanceRaw, err := json.Marshal(acceptance)
	if err != nil {
		return documentV2{}, nil, err
	}
	document := documentV2{
		Schema: HandoffSchemaV2, Mode: ModeNative, Engine: "summer", ProjectID: strings.TrimSpace(state.ProjectID),
		ObjectiveID: strings.TrimSpace(state.ObjectiveID), ObjectiveStatus: strings.TrimSpace(state.ObjectiveStatus),
		ObjectiveRevision: state.ObjectiveRevision, LedgerRevision: cursor.Revision, LedgerHead: cursor.Digest,
		ProjectorVersion: ProjectorVersion, BuiltAt: state.BuiltAt.UTC(), Goal: strings.TrimSpace(state.Goal),
		Profile: strings.TrimSpace(state.Profile), AcceptanceCount: len(acceptance),
		AcceptanceDigest: fmt.Sprintf("%x", sha256.Sum256(acceptanceRaw)),
		Done:             nonNil(cleanValues(state.Done)), Next: nonNil(cleanValues(state.Next)), Validation: nonNil(cleanValues(state.Validation)),
		Blockers: nonNil(cleanValues(state.Blockers)), MustRead: nonNil(cleanValues(state.MustRead)), ResumeCommand: state.ResumeCommand,
	}
	if document.ResumeCommand == "" {
		document.ResumeCommand = "summer resume"
	}
	if err := validateDocumentV2(document); err != nil {
		return documentV2{}, nil, err
	}
	digestRaw, err := documentDigestInput(document)
	if err != nil {
		return documentV2{}, nil, err
	}
	document.ContentDigest = fmt.Sprintf("%x", sha256.Sum256(digestRaw))
	meta, err := structToMap(document)
	if err != nil {
		return documentV2{}, nil, err
	}
	raw, err := renderMarkdown(meta, "Project Handoff", []section{
		{heading: "当前目标", values: []string{document.Goal}},
		{heading: "已完成", values: document.Done},
		{heading: "唯一下一步", values: document.Next},
		{heading: "验证", values: document.Validation},
		{heading: "阻塞", values: document.Blockers},
		{heading: "必须读取", values: document.MustRead},
	})
	return document, raw, err
}

func capsuleV2(state State, cursor Cursor) Capsule {
	return Capsule{
		Schema: CapsuleSchemaV2, Mode: ModeNative, Engine: "summer", Goal: strings.TrimSpace(state.Goal),
		Done: nonNil(cleanValues(state.Done)), Next: nonNil(cleanValues(state.Next)),
		Validation: nonNil(cleanValues(state.Validation)), Blockers: nonNil(cleanValues(state.Blockers)),
		MustRead: nonNil(cleanValues(state.MustRead)), SourcePath: ".agent/ledger/HEAD",
		ResumeCommand: state.ResumeCommand, Status: strings.TrimSpace(state.ObjectiveStatus),
		Revision: state.ObjectiveRevision, Profile: strings.TrimSpace(state.Profile),
		Acceptance: nonNil(cleanValues(state.Acceptance)), ProjectID: strings.TrimSpace(state.ProjectID),
		ObjectiveID: strings.TrimSpace(state.ObjectiveID), LedgerRevision: cursor.Revision, LedgerHead: cursor.Digest,
	}
}

func validateDocumentV2(document documentV2) error {
	if document.Schema != HandoffSchemaV2 || document.Mode != ModeNative || document.Engine != "summer" {
		return errors.New("v2 handoff identity is invalid")
	}
	if document.ProjectID == "" || document.ObjectiveID == "" || document.ObjectiveStatus == "" || document.ObjectiveRevision == 0 || document.LedgerRevision == 0 || document.LedgerHead == "" || document.ProjectorVersion != ProjectorVersion || document.BuiltAt.IsZero() || strings.TrimSpace(document.Goal) == "" || strings.TrimSpace(document.Profile) == "" || document.AcceptanceCount == 0 || len(document.AcceptanceDigest) != sha256.Size*2 {
		return errors.New("v2 handoff is incomplete")
	}
	if len(document.MustRead) > 5 || len(document.Done) > 8 || len(document.Next) > 3 || len(document.Validation) > 8 || len(document.Blockers) > 5 {
		return errors.New("v2 handoff collection exceeds its item limit")
	}
	for _, value := range append(append(append(append(append([]string{document.ProjectID, document.ObjectiveID, document.ObjectiveStatus, document.Goal, document.Profile, document.ResumeCommand}, document.Done...), document.Next...), document.Validation...), document.Blockers...), document.MustRead...) {
		if utf8.RuneCountInString(value) > 2000 || strings.ContainsAny(value, "\r\n") || containsContinuitySecret(value) {
			return errors.New("v2 handoff contains oversized or sensitive content")
		}
	}
	if document.ContentDigest != "" {
		raw, err := documentDigestInput(document)
		if err != nil {
			return err
		}
		if document.ContentDigest != fmt.Sprintf("%x", sha256.Sum256(raw)) {
			return errors.New("v2 handoff content digest is invalid")
		}
	}
	return nil
}

func documentDigestInput(document documentV2) ([]byte, error) {
	value, err := structToMap(document)
	if err != nil {
		return nil, err
	}
	delete(value, "content_digest")
	return json.Marshal(value)
}

func containsContinuitySecret(value string) bool {
	if strings.Contains(value, "-----BEGIN PRIVATE KEY-----") || strings.Contains(value, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		return true
	}
	for _, pattern := range continuitySecretPatterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func (m *Module) ensureProjectionDirectories() error {
	agent := filepath.Join(m.root, ".agent")
	for _, path := range []string{agent, filepath.Join(agent, "runtime")} {
		if info, err := os.Lstat(path); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return wrap(CodeUnsafeReference, "prepare projection", path, errors.New("projection directory is not a regular directory"))
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return wrap(CodeProjectionConflict, "prepare projection", path, err)
			}
		} else {
			return wrap(CodeProjectionConflict, "prepare projection", path, err)
		}
	}
	if info, err := os.Lstat(m.handoffPath); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return wrap(CodeUnsafeReference, "prepare projection", ".agent/HANDOFF.md", errors.New("handoff is not a regular file"))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrap(CodeProjectionConflict, "prepare projection", ".agent/HANDOFF.md", err)
	}
	return nil
}

func writeAtomicFile(path string, raw []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".HANDOFF-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return fsyncProjectionDirectory(directory)
}
