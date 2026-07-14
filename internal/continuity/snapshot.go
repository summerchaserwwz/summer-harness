package continuity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	resumeSnapshotSchema = "summer.resume-snapshot/v1"
	resumeSnapshotLimit  = 64 << 10
)

type resumeSnapshot struct {
	Schema           string        `json:"schema"`
	ProjectID        string        `json:"project_id"`
	LedgerRevision   uint64        `json:"ledger_revision"`
	LedgerHead       string        `json:"ledger_head"`
	ProjectorVersion int           `json:"projector_version"`
	BuiltAt          time.Time     `json:"built_at"`
	StateDigest      string        `json:"state_digest"`
	State            snapshotState `json:"state"`
	ContentDigest    string        `json:"content_digest"`
}

type snapshotState struct {
	ProjectID         string    `json:"project_id"`
	ObjectiveID       string    `json:"objective_id"`
	ObjectiveStatus   string    `json:"objective_status"`
	ObjectiveRevision uint64    `json:"objective_revision"`
	Goal              string    `json:"goal"`
	Profile           string    `json:"profile"`
	Acceptance        []string  `json:"acceptance"`
	Done              []string  `json:"done"`
	Next              []string  `json:"next"`
	Validation        []string  `json:"validation"`
	Blockers          []string  `json:"blockers"`
	MustRead          []string  `json:"must_read"`
	ResumeCommand     string    `json:"resume_command"`
	BuiltAt           time.Time `json:"built_at"`
}

func (m *Module) snapshotOrSource(ctx context.Context, projectID string, source Source) (State, Cursor, error) {
	current, err := source.Head(ctx, projectID)
	if err != nil {
		return State{}, Cursor{}, err
	}
	return m.snapshotOrSourceAt(ctx, projectID, source, current)
}

func (m *Module) snapshotOrSourceAt(ctx context.Context, projectID string, source Source, current Cursor) (State, Cursor, error) {
	if state, found, loadErr := m.loadSnapshot(projectID, current); loadErr != nil {
		return State{}, Cursor{}, loadErr
	} else if found {
		return state, current, nil
	}
	state, cursor, err := source.Snapshot(ctx, projectID)
	if err != nil {
		return State{}, Cursor{}, err
	}
	if cursor != current {
		return State{}, Cursor{}, wrap(CodeProjectionStale, "rebuild resume snapshot", ".agent/cache/resume.snapshot.json", errors.New("ledger changed while rebuilding snapshot"))
	}
	_ = m.projectSnapshot(ctx, state, cursor)
	return state, cursor, nil
}

func (m *Module) projectSnapshot(ctx context.Context, state State, cursor Cursor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	document, raw, err := buildResumeSnapshot(state, cursor)
	if err != nil {
		return wrap(CodeProjectionConflict, "build resume snapshot", ".agent/cache/resume.snapshot.json", err)
	}
	if len(raw) > resumeSnapshotLimit {
		return wrap(CodeProjectionConflict, "build resume snapshot", ".agent/cache/resume.snapshot.json", fmt.Errorf("snapshot exceeds %d bytes", resumeSnapshotLimit))
	}
	if err := m.ensureSnapshotDirectory(); err != nil {
		return err
	}
	lockPath := filepath.Join(m.root, ".agent", "runtime", "snapshot.write.lock")
	unlock, err := acquireProjectionLock(ctx, lockPath)
	if err != nil {
		return wrap(CodeProjectionConflict, "lock resume snapshot", ".agent/runtime/snapshot.write.lock", err)
	}
	defer unlock()
	path := m.snapshotPath()
	if existing, readErr := readRegularFile(path, resumeSnapshotLimit); readErr == nil {
		var current resumeSnapshot
		if decodeStrictJSON(existing, &current) == nil && current.ProjectID == document.ProjectID && current.LedgerRevision > document.LedgerRevision {
			return wrap(CodeProjectionStale, "publish resume snapshot", ".agent/cache/resume.snapshot.json", errors.New("newer snapshot already exists"))
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		info, statErr := os.Lstat(path)
		if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return wrap(CodeUnsafeReference, "inspect resume snapshot", ".agent/cache/resume.snapshot.json", errors.New("snapshot is not a regular file"))
		}
	}
	if err := writeAtomicFile(path, raw, 0o600); err != nil {
		return wrap(CodeProjectionConflict, "publish resume snapshot", ".agent/cache/resume.snapshot.json", err)
	}
	return nil
}

func (m *Module) loadSnapshot(projectID string, cursor Cursor) (State, bool, error) {
	if cursor.ResumeDigest == "" {
		return State{}, false, nil
	}
	path := m.snapshotPath()
	cache := filepath.Dir(path)
	if info, err := os.Lstat(cache); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return State{}, false, wrap(CodeUnsafeReference, "read resume snapshot", ".agent/cache", errors.New("cache is not a regular directory"))
		}
	} else if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	} else {
		return State{}, false, wrap(CodeProjectionConflict, "inspect resume snapshot", ".agent/cache", err)
	}
	raw, err := readRegularFile(path, resumeSnapshotLimit)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, errTooLarge) {
		return State{}, false, nil
	}
	if err != nil {
		info, statErr := os.Lstat(path)
		if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return State{}, false, wrap(CodeUnsafeReference, "read resume snapshot", ".agent/cache/resume.snapshot.json", errors.New("snapshot is not a regular file"))
		}
		return State{}, false, nil
	}
	var document resumeSnapshot
	if err := decodeStrictJSON(raw, &document); err != nil {
		return State{}, false, nil
	}
	if document.Schema != resumeSnapshotSchema || document.ProjectID != projectID || document.LedgerRevision != cursor.Revision || document.LedgerHead != cursor.Digest || document.ProjectorVersion != ProjectorVersion || document.BuiltAt.IsZero() {
		return State{}, false, nil
	}
	state := document.State.toState()
	stateDigest, err := CanonicalResumeDigest(state)
	if err != nil || document.StateDigest != cursor.ResumeDigest || document.StateDigest != stateDigest {
		return State{}, false, nil
	}
	contentRaw, err := snapshotDigestInput(document)
	if err != nil || document.ContentDigest != fmt.Sprintf("%x", sha256.Sum256(contentRaw)) {
		return State{}, false, nil
	}
	if _, _, err := buildDocumentV2(state, cursor); err != nil {
		return State{}, false, nil
	}
	if err := checkCapsuleSize(capsuleV2(state, cursor)); err != nil {
		return State{}, false, nil
	}
	return state, true, nil
}

func buildResumeSnapshot(state State, cursor Cursor) (resumeSnapshot, []byte, error) {
	canonical := snapshotStateFrom(state)
	stateDigest, err := CanonicalResumeDigest(canonical.toState())
	if err != nil {
		return resumeSnapshot{}, nil, err
	}
	if cursor.ResumeDigest != "" && cursor.ResumeDigest != stateDigest {
		return resumeSnapshot{}, nil, errors.New("canonical resume digest does not match snapshot state")
	}
	document := resumeSnapshot{
		Schema: resumeSnapshotSchema, ProjectID: canonical.ProjectID,
		LedgerRevision: cursor.Revision, LedgerHead: cursor.Digest,
		ProjectorVersion: ProjectorVersion, BuiltAt: canonical.BuiltAt,
		StateDigest: stateDigest, State: canonical,
	}
	if document.ProjectID == "" || document.LedgerRevision == 0 || len(document.LedgerHead) != sha256.Size*2 || document.BuiltAt.IsZero() {
		return resumeSnapshot{}, nil, errors.New("snapshot identity is incomplete")
	}
	if _, _, err := buildDocumentV2(canonical.toState(), cursor); err != nil {
		return resumeSnapshot{}, nil, err
	}
	digestRaw, err := snapshotDigestInput(document)
	if err != nil {
		return resumeSnapshot{}, nil, err
	}
	document.ContentDigest = fmt.Sprintf("%x", sha256.Sum256(digestRaw))
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return resumeSnapshot{}, nil, err
	}
	return document, append(raw, '\n'), nil
}

// CanonicalResumeDigest binds the actionable continuity state to the canonical
// transaction without depending on the commit timestamp used by projections.
func CanonicalResumeDigest(state State) (string, error) {
	canonical := snapshotStateFrom(state)
	canonical.BuiltAt = time.Time{}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func snapshotStateFrom(state State) snapshotState {
	resumeCommand := strings.TrimSpace(state.ResumeCommand)
	if resumeCommand == "" {
		resumeCommand = "summer resume"
	}
	return snapshotState{
		ProjectID: strings.TrimSpace(state.ProjectID), ObjectiveID: strings.TrimSpace(state.ObjectiveID),
		ObjectiveStatus: strings.TrimSpace(state.ObjectiveStatus), ObjectiveRevision: state.ObjectiveRevision,
		Goal: strings.TrimSpace(state.Goal), Profile: strings.TrimSpace(state.Profile),
		Acceptance: nonNil(cleanValues(state.Acceptance)), Done: nonNil(cleanValues(state.Done)),
		Next: nonNil(cleanValues(state.Next)), Validation: nonNil(cleanValues(state.Validation)),
		Blockers: nonNil(cleanValues(state.Blockers)), MustRead: nonNil(cleanValues(state.MustRead)),
		ResumeCommand: resumeCommand, BuiltAt: state.BuiltAt.UTC(),
	}
}

func (state snapshotState) toState() State {
	return State{
		ProjectID: state.ProjectID, ObjectiveID: state.ObjectiveID, ObjectiveStatus: state.ObjectiveStatus,
		ObjectiveRevision: state.ObjectiveRevision, Goal: state.Goal, Profile: state.Profile,
		Acceptance: nonNil(state.Acceptance), Done: nonNil(state.Done), Next: nonNil(state.Next),
		Validation: nonNil(state.Validation), Blockers: nonNil(state.Blockers), MustRead: nonNil(state.MustRead),
		ResumeCommand: state.ResumeCommand, BuiltAt: state.BuiltAt,
	}
}

func snapshotDigestInput(document resumeSnapshot) ([]byte, error) {
	document.ContentDigest = ""
	return json.Marshal(document)
}

func (m *Module) ensureSnapshotDirectory() error {
	cache := filepath.Join(m.root, ".agent", "cache")
	if info, err := os.Lstat(cache); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return wrap(CodeUnsafeReference, "prepare resume snapshot", ".agent/cache", errors.New("cache is not a regular directory"))
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return wrap(CodeProjectionConflict, "prepare resume snapshot", ".agent/cache", err)
	}
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return wrap(CodeProjectionConflict, "prepare resume snapshot", ".agent/cache", err)
	}
	return fsyncProjectionDirectory(filepath.Dir(cache))
}

func (m *Module) snapshotPath() string {
	return filepath.Join(m.root, ".agent", "cache", "resume.snapshot.json")
}
