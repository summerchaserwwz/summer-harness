package continuity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const legacySchema = "summer-harness/v1"

type Module struct {
	root        string
	handoffPath string
}

type legacyHandoff struct {
	Schema        string   `json:"schema"`
	Mode          Mode     `json:"mode"`
	Engine        string   `json:"engine"`
	Goal          string   `json:"goal"`
	Done          []string `json:"done"`
	Next          []string `json:"next"`
	Validation    []string `json:"validation"`
	Blockers      []string `json:"blockers"`
	MustRead      []string `json:"must_read"`
	SourcePath    string   `json:"source_path"`
	SourceDigest  string   `json:"source_digest"`
	TaskID        string   `json:"task_id"`
	TaskStatus    string   `json:"task_status"`
	ResumeCommand string   `json:"resume_command"`
	UpdatedAt     string   `json:"updated_at"`
	LastWriter    string   `json:"last_writer"`
}

type legacyTask struct {
	Schema             string       `json:"schema"`
	Kind               string       `json:"kind"`
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Goal               string       `json:"goal"`
	Acceptance         []string     `json:"acceptance"`
	Status             string       `json:"status"`
	Profile            string       `json:"profile"`
	Risk               string       `json:"risk"`
	Revision           uint64       `json:"revision"`
	ValidationRevision uint64       `json:"validation_revision"`
	Done               []string     `json:"done"`
	Next               []string     `json:"next"`
	Validation         []string     `json:"validation"`
	Blockers           []string     `json:"blockers"`
	MustRead           []string     `json:"must_read"`
	ResidualRisks      []string     `json:"residual_risks"`
	Engine             string       `json:"engine"`
	CreatedAt          string       `json:"created_at"`
	CreatedBy          string       `json:"created_by"`
	UpdatedAt          string       `json:"updated_at"`
	LastWriter         string       `json:"last_writer"`
	LastWorkSession    string       `json:"last_work_session"`
	Review             legacyReview `json:"review"`
}

type legacyReview struct {
	Approved         bool     `json:"approved"`
	Findings         []string `json:"findings"`
	ReviewedRevision uint64   `json:"reviewed_revision"`
	Summary          string   `json:"summary"`
}

var legacyTaskIDPattern = regexp.MustCompile(`^task_[A-Za-z0-9_-]+$`)

func NewFile(root string) (*Module, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", resolved)
	}
	return &Module{root: resolved, handoffPath: filepath.Join(resolved, ".agent", "HANDOFF.md")}, nil
}

func (m *Module) Resume(ctx context.Context, source Source) (Capsule, error) {
	if err := ctx.Err(); err != nil {
		return Capsule{}, err
	}
	canonicalProject := ""
	canonicalFound := false
	if source != nil {
		var err error
		canonicalProject, canonicalFound, err = source.Project(ctx)
		if err != nil {
			return Capsule{}, err
		}
	}
	raw, err := m.readHandoff()
	if ErrorCode(err) == CodeHandoffNotFound && canonicalFound {
		state, cursor, snapshotErr := source.Snapshot(ctx, canonicalProject)
		if snapshotErr != nil {
			return Capsule{}, snapshotErr
		}
		if _, projectErr := m.Project(ctx, state, cursor); projectErr != nil {
			return Capsule{}, projectErr
		}
		raw, err = m.readHandoff()
	}
	if err != nil {
		return Capsule{}, err
	}
	metaRaw, err := splitMarkdown(raw)
	if err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "parse handoff", ".agent/HANDOFF.md", err)
	}
	schema, err := readSchema(metaRaw)
	if err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "decode handoff schema", ".agent/HANDOFF.md", err)
	}
	if schema == HandoffSchemaV2 {
		return m.resumeV2(ctx, raw, metaRaw, source)
	}
	if schema != legacySchema {
		return Capsule{}, wrap(CodeHandoffUnsupportedSchema, "decode handoff", ".agent/HANDOFF.md", fmt.Errorf("unsupported schema %q", schema))
	}
	if canonicalFound {
		return Capsule{}, wrap(CodeLifecycleConflict, "resume lifecycle", ".agent/HANDOFF.md", errors.New("legacy handoff conflicts with canonical v2 ledger"))
	}
	var handoff legacyHandoff
	if err := decodeStrictJSON(metaRaw, &handoff); err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "decode handoff", ".agent/HANDOFF.md", err)
	}
	if err := validateLegacyHandoff(handoff); err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "validate handoff", ".agent/HANDOFF.md", err)
	}
	expected, err := renderLegacyHandoff(handoff)
	if err != nil {
		return Capsule{}, wrap(CodeHandoffInvalid, "render handoff", ".agent/HANDOFF.md", err)
	}
	if !bytes.Equal(raw, expected) {
		return Capsule{}, wrap(CodeHandoffDrift, "validate handoff body", ".agent/HANDOFF.md", errors.New("frontmatter and Markdown body differ"))
	}
	capsule, err := m.resumeLegacy(handoff)
	if err != nil {
		return Capsule{}, err
	}
	if handoff.Mode == ModeNative || handoff.Mode == ModeGSD {
		after, readErr := m.readHandoff()
		if readErr != nil {
			return Capsule{}, readErr
		}
		if !bytes.Equal(raw, after) {
			return Capsule{}, wrap(CodeHandoffDrift, "resume handoff", ".agent/HANDOFF.md", errors.New("handoff changed during resume"))
		}
	}
	if err := checkCapsuleSize(capsule); err != nil {
		return Capsule{}, err
	}
	return capsule, nil
}

func (m *Module) resumeLegacy(handoff legacyHandoff) (Capsule, error) {
	capsule := Capsule{
		Schema: legacySchema, Mode: handoff.Mode, Engine: handoff.Engine, Goal: handoff.Goal,
		Done: nonNil(handoff.Done), Next: nonNil(handoff.Next), Validation: nonNil(handoff.Validation),
		Blockers: nonNil(handoff.Blockers), MustRead: nonNil(handoff.MustRead), SourcePath: handoff.SourcePath,
		ResumeCommand: handoff.ResumeCommand,
	}
	switch handoff.Mode {
	case ModeDirect, ModeIdle:
		for _, reference := range handoff.MustRead {
			if err := m.validateReference(reference, ""); err != nil {
				return Capsule{}, err
			}
		}
		return capsule, nil
	case ModeNative:
		task, err := m.readLegacyNativeTask(handoff)
		if err != nil {
			return Capsule{}, err
		}
		if err := compareLegacyProjection(handoff, task); err != nil {
			return Capsule{}, wrap(CodeHandoffDrift, "validate native projection", ".agent/HANDOFF.md", err)
		}
		for _, reference := range task.MustRead {
			if err := m.validateReference(reference, ""); err != nil {
				return Capsule{}, err
			}
		}
		decisions, facts, err := m.readLegacyMemory(task.ID)
		if err != nil {
			return Capsule{}, err
		}
		capsule.Goal = task.Goal
		capsule.Done = nonNil(task.Done)
		capsule.Next = nonNil(task.Next)
		capsule.Validation = nonNil(task.Validation)
		capsule.Blockers = nonNil(task.Blockers)
		capsule.MustRead = nonNil(task.MustRead)
		capsule.TaskID = task.ID
		capsule.Status = task.Status
		capsule.Profile = task.Profile
		capsule.Risk = task.Risk
		capsule.Revision = task.Revision
		capsule.Acceptance = nonNil(task.Acceptance)
		return limitLegacyMemory(capsule, decisions, facts)
	case ModeGSD:
		if handoff.TaskID != "" || handoff.TaskStatus != "" || handoff.ResumeCommand == "" {
			return Capsule{}, wrap(CodeHandoffInvalid, "validate GSD handoff", ".agent/HANDOFF.md", errors.New("GSD handoff contains invalid task fields"))
		}
		if err := m.validateReference(handoff.SourcePath, ".planning"); err != nil {
			return Capsule{}, err
		}
		sourcePath := filepath.Join(m.root, filepath.FromSlash(handoff.SourcePath))
		before, err := readRegularFile(sourcePath, 1<<20)
		if err != nil {
			return Capsule{}, wrap(CodeUnsafeReference, "read GSD pointer", handoff.SourcePath, err)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(before))
		if digest != handoff.SourceDigest {
			return Capsule{}, wrap(CodeGSDPointerStale, "validate GSD pointer", handoff.SourcePath, errors.New("GSD source digest differs from handoff"))
		}
		after, err := readRegularFile(sourcePath, 1<<20)
		if err != nil || !bytes.Equal(before, after) {
			return Capsule{}, wrap(CodeGSDPointerStale, "validate GSD pointer", handoff.SourcePath, errors.New("GSD source changed during resume"))
		}
		for _, reference := range handoff.MustRead {
			if err := m.validateReference(reference, ""); err != nil {
				return Capsule{}, err
			}
		}
		return capsule, nil
	default:
		return Capsule{}, wrap(CodeHandoffInvalid, "resume handoff", ".agent/HANDOFF.md", fmt.Errorf("unsupported mode %q", handoff.Mode))
	}
}

func (m *Module) readLegacyNativeTask(handoff legacyHandoff) (legacyTask, error) {
	if !legacyTaskIDPattern.MatchString(handoff.TaskID) {
		return legacyTask{}, wrap(CodeHandoffInvalid, "validate task id", ".agent/HANDOFF.md", errors.New("task id is invalid"))
	}
	expectedSource := filepath.ToSlash(filepath.Join(".agent", "ledger", "tasks", handoff.TaskID+".md"))
	if handoff.SourcePath != expectedSource {
		return legacyTask{}, wrap(CodeUnsafeReference, "validate native source", handoff.SourcePath, errors.New("source path does not match task id"))
	}
	if err := m.validateReference(expectedSource, ".agent"); err != nil {
		return legacyTask{}, err
	}
	raw, err := readRegularFile(filepath.Join(m.root, filepath.FromSlash(expectedSource)), 256<<10)
	if err != nil {
		return legacyTask{}, wrap(CodeHandoffNotFound, "read native task", expectedSource, err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if digest != handoff.SourceDigest {
		return legacyTask{}, wrap(CodeHandoffDrift, "validate native task digest", expectedSource, errors.New("task digest differs from handoff"))
	}
	metaRaw, err := splitMarkdown(raw)
	if err != nil {
		return legacyTask{}, wrap(CodeHandoffInvalid, "parse native task", expectedSource, err)
	}
	var task legacyTask
	if err := decodeStrictJSON(metaRaw, &task); err != nil {
		return legacyTask{}, wrap(CodeHandoffInvalid, "decode native task", expectedSource, err)
	}
	if task.Schema != legacySchema || task.Kind != "task" || task.ID != handoff.TaskID || task.Engine != "summer" {
		return legacyTask{}, wrap(CodeHandoffInvalid, "validate native task", expectedSource, errors.New("task identity is invalid"))
	}
	if task.Status != "active" && task.Status != "blocked" && task.Status != "review" {
		return legacyTask{}, wrap(CodeHandoffDrift, "validate native task", expectedSource, fmt.Errorf("task status %q is not active", task.Status))
	}
	expected, err := renderLegacyTask(task)
	if err != nil {
		return legacyTask{}, wrap(CodeHandoffInvalid, "render native task", expectedSource, err)
	}
	if !bytes.Equal(raw, expected) {
		return legacyTask{}, wrap(CodeHandoffDrift, "validate native task body", expectedSource, errors.New("task frontmatter and Markdown body differ"))
	}
	if err := m.validateOnlyActiveTask(task.ID); err != nil {
		return legacyTask{}, err
	}
	return task, nil
}

func (m *Module) validateOnlyActiveTask(expectedID string) error {
	directory := filepath.Join(m.root, ".agent", "ledger", "tasks")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return wrap(CodeHandoffInvalid, "scan native tasks", ".agent/ledger/tasks", err)
	}
	if len(entries) > 10_000 {
		return wrap(CodeHandoffInvalid, "scan native tasks", ".agent/ledger/tasks", errors.New("task directory exceeds entry limit"))
	}
	active := make([]string, 0, 1)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return wrap(CodeUnsafeReference, "scan native tasks", filepath.ToSlash(filepath.Join(".agent", "ledger", "tasks", entry.Name())), errors.New("task entry is not a regular file"))
		}
		raw, err := readRegularFile(filepath.Join(directory, entry.Name()), 256<<10)
		if err != nil {
			return wrap(CodeHandoffInvalid, "scan native tasks", entry.Name(), err)
		}
		metaRaw, err := splitMarkdown(raw)
		if err != nil {
			return wrap(CodeHandoffInvalid, "scan native tasks", entry.Name(), err)
		}
		var task legacyTask
		if err := decodeStrictJSON(metaRaw, &task); err != nil {
			return wrap(CodeHandoffInvalid, "scan native tasks", entry.Name(), err)
		}
		if task.Status == "active" || task.Status == "blocked" || task.Status == "review" {
			active = append(active, task.ID)
		}
	}
	if len(active) != 1 || active[0] != expectedID {
		return wrap(CodeHandoffDrift, "validate active task", ".agent/ledger/tasks", errors.New("active task does not match the one handoff"))
	}
	return nil
}

func compareLegacyProjection(handoff legacyHandoff, task legacyTask) error {
	checks := map[string]bool{
		"mode": handoff.Mode == ModeNative, "engine": handoff.Engine == "summer",
		"task_id": handoff.TaskID == task.ID, "task_status": handoff.TaskStatus == task.Status,
		"goal": handoff.Goal == task.Goal, "done": equalStrings(handoff.Done, task.Done),
		"next": equalStrings(handoff.Next, task.Next), "validation": equalStrings(handoff.Validation, task.Validation),
		"blockers": equalStrings(handoff.Blockers, task.Blockers), "must_read": equalStrings(handoff.MustRead, task.MustRead),
		"resume_command": handoff.ResumeCommand == "$project-handoff",
	}
	for field, valid := range checks {
		if !valid {
			return fmt.Errorf("field %s differs from canonical task", field)
		}
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func renderLegacyTask(task legacyTask) ([]byte, error) {
	var meta map[string]any
	raw, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	return renderMarkdown(meta, task.Title, []section{
		{heading: "目标", values: []string{task.Goal}},
		{heading: "验收条件", values: task.Acceptance},
		{heading: "已完成", values: task.Done},
		{heading: "下一步", values: task.Next},
		{heading: "验证", values: task.Validation},
		{heading: "阻塞", values: task.Blockers},
		{heading: "必须读取", values: task.MustRead},
		{heading: "残余风险", values: task.ResidualRisks},
	})
}

func readSchema(raw []byte) (string, error) {
	var value map[string]json.RawMessage
	if err := decodeStrictJSON(raw, &value); err != nil {
		return "", err
	}
	schemaRaw, exists := value["schema"]
	if !exists {
		return "", errors.New("schema is required")
	}
	var schema string
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return "", err
	}
	return schema, nil
}

func (m *Module) readHandoff() ([]byte, error) {
	agent := filepath.Join(m.root, ".agent")
	info, err := os.Lstat(agent)
	if errors.Is(err, os.ErrNotExist) {
		return nil, wrap(CodeHandoffNotFound, "read handoff", ".agent/HANDOFF.md", err)
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, wrap(CodeUnsafeReference, "inspect .agent", ".agent", fmt.Errorf(".agent is not a regular directory"))
	}
	handoffInfo, err := os.Lstat(m.handoffPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, wrap(CodeHandoffNotFound, "read handoff", ".agent/HANDOFF.md", err)
	}
	if err != nil {
		return nil, wrap(CodeHandoffInvalid, "inspect handoff", ".agent/HANDOFF.md", err)
	}
	if handoffInfo.Mode()&os.ModeSymlink != 0 || !handoffInfo.Mode().IsRegular() {
		return nil, wrap(CodeUnsafeReference, "inspect handoff", ".agent/HANDOFF.md", errors.New("handoff is not a regular file"))
	}
	raw, err := readRegularFile(m.handoffPath, HandoffLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, wrap(CodeHandoffNotFound, "read handoff", ".agent/HANDOFF.md", err)
	}
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return nil, wrap(CodeHandoffTooLarge, "read handoff", ".agent/HANDOFF.md", err)
		}
		return nil, wrap(CodeHandoffInvalid, "read handoff", ".agent/HANDOFF.md", err)
	}
	return raw, nil
}

var errTooLarge = errors.New("file exceeds byte limit")

func readRegularFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("file changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, errTooLarge
	}
	return raw, nil
}

func splitMarkdown(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return nil, errors.New("missing opening frontmatter fence")
	}
	end := bytes.Index(raw[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, errors.New("missing closing frontmatter fence")
	}
	return raw[4 : 4+end], nil
}

func validateLegacyHandoff(handoff legacyHandoff) error {
	if handoff.Schema != legacySchema {
		return errors.New("legacy handoff schema is invalid")
	}
	if len(handoff.MustRead) > 5 || len(handoff.Done) > 8 || len(handoff.Next) > 3 || len(handoff.Validation) > 8 || len(handoff.Blockers) > 5 {
		return errors.New("handoff collection exceeds its item limit")
	}
	for _, value := range append(append(append(append(append([]string{handoff.Goal}, handoff.Done...), handoff.Next...), handoff.Validation...), handoff.Blockers...), handoff.MustRead...) {
		if len(value) > 2000 {
			return errors.New("handoff text exceeds its character limit")
		}
	}
	switch handoff.Mode {
	case ModeDirect:
		if handoff.Engine != "direct" || handoff.SourcePath != "" || handoff.SourceDigest != "" || handoff.TaskID != "" || handoff.TaskStatus != "" {
			return errors.New("direct handoff identity is invalid")
		}
	case ModeNative:
		if handoff.Engine != "summer" || handoff.TaskID == "" || handoff.SourcePath == "" || handoff.SourceDigest == "" {
			return errors.New("native handoff identity is incomplete")
		}
	case ModeGSD:
		if handoff.Engine != "gsd" || handoff.SourcePath == "" || handoff.SourceDigest == "" {
			return errors.New("GSD handoff identity is incomplete")
		}
	case ModeIdle:
		if handoff.Engine != "none" {
			return errors.New("idle handoff identity is invalid")
		}
	default:
		return fmt.Errorf("unsupported handoff mode %q", handoff.Mode)
	}
	return nil
}

func renderLegacyHandoff(handoff legacyHandoff) ([]byte, error) {
	var meta map[string]any
	raw, err := json.Marshal(handoff)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	return renderMarkdown(meta, "Project Handoff", []section{
		{heading: "当前目标", values: []string{handoff.Goal}},
		{heading: "已完成", values: handoff.Done},
		{heading: "唯一下一步", values: handoff.Next},
		{heading: "验证", values: handoff.Validation},
		{heading: "阻塞", values: handoff.Blockers},
		{heading: "必须读取", values: handoff.MustRead},
	})
}

type section struct {
	heading string
	values  []string
}

func renderMarkdown(meta map[string]any, title string, sections []section) ([]byte, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(meta); err != nil {
		return nil, err
	}
	var output strings.Builder
	output.WriteString("---\n")
	output.Write(encoded.Bytes())
	output.WriteString("---\n# ")
	output.WriteString(title)
	output.WriteString("\n\n")
	for _, section := range sections {
		values := cleanValues(section.values)
		if len(values) == 0 {
			continue
		}
		output.WriteString("## ")
		output.WriteString(section.heading)
		output.WriteString("\n\n")
		for _, value := range values {
			output.WriteString("- ")
			output.WriteString(value)
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n"), nil
}

func cleanValues(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func checkCapsuleSize(capsule Capsule) error {
	raw, err := json.MarshalIndent(capsule, "", "  ")
	if err != nil {
		return wrap(CodeHandoffInvalid, "encode capsule", "", err)
	}
	if len(raw) > CapsuleLimit {
		return wrap(CodeCapsuleTooLarge, "encode capsule", "", fmt.Errorf("capsule exceeds %d bytes", CapsuleLimit))
	}
	return nil
}

func (m *Module) validateReference(reference, requiredPrefix string) error {
	if reference == "" || !filepath.IsLocal(reference) || filepath.IsAbs(reference) || filepath.Clean(reference) != reference {
		return wrap(CodeUnsafeReference, "validate reference", reference, errors.New("reference must be a clean repository-relative path"))
	}
	parts := strings.Split(filepath.ToSlash(reference), "/")
	if requiredPrefix != "" && (len(parts) == 0 || parts[0] != requiredPrefix) {
		return wrap(CodeUnsafeReference, "validate reference", reference, fmt.Errorf("reference must be under %s", requiredPrefix))
	}
	current := m.root
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return wrap(CodeUnsafeReference, "validate reference", reference, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return wrap(CodeUnsafeReference, "validate reference", reference, errors.New("reference passes through a symlink"))
		}
	}
	info, err := os.Stat(current)
	if err != nil || !info.Mode().IsRegular() {
		return wrap(CodeUnsafeReference, "validate reference", reference, errors.New("reference is not a regular file"))
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return wrap(CodeUnsafeReference, "validate reference", reference, err)
	}
	relative, err := filepath.Rel(m.root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return wrap(CodeUnsafeReference, "validate reference", reference, errors.New("reference escapes the repository"))
	}
	return nil
}
