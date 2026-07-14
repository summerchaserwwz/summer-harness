package continuity

import (
	"context"
	"time"
)

const (
	HandoffLimit     = 4 << 10
	CapsuleLimit     = 32 << 10
	HandoffSchemaV2  = "summer.handoff/v2"
	CapsuleSchemaV2  = "summer.capsule/v2"
	ProjectorVersion = 1
)

type Mode string

const (
	ModeIdle   Mode = "idle"
	ModeDirect Mode = "direct"
	ModeNative Mode = "native"
	ModeGSD    Mode = "gsd"
)

type Capsule struct {
	Schema         string            `json:"schema"`
	Mode           Mode              `json:"mode"`
	Engine         string            `json:"engine"`
	Goal           string            `json:"goal"`
	Done           []string          `json:"done"`
	Next           []string          `json:"next"`
	Validation     []string          `json:"validation"`
	Blockers       []string          `json:"blockers"`
	MustRead       []string          `json:"must_read"`
	SourcePath     string            `json:"source_path"`
	ResumeCommand  string            `json:"resume_command"`
	TaskID         string            `json:"task_id,omitempty"`
	Status         string            `json:"status,omitempty"`
	Profile        string            `json:"profile,omitempty"`
	Risk           string            `json:"risk,omitempty"`
	Revision       uint64            `json:"revision,omitempty"`
	Acceptance     []string          `json:"acceptance,omitempty"`
	Decisions      *[]map[string]any `json:"decisions,omitempty"`
	Facts          *[]map[string]any `json:"facts,omitempty"`
	Omitted        *Omitted          `json:"omitted,omitempty"`
	ProjectID      string            `json:"project_id,omitempty"`
	ObjectiveID    string            `json:"objective_id,omitempty"`
	LedgerRevision uint64            `json:"ledger_revision,omitempty"`
	LedgerHead     string            `json:"ledger_head,omitempty"`
	ResumeDigest   string            `json:"resume_digest,omitempty"`
}

type Omitted struct {
	Decisions int `json:"decisions"`
	Facts     int `json:"facts"`
}

type Cursor struct {
	Revision     uint64
	Digest       string
	ResumeDigest string
}

type State struct {
	ProjectID         string
	ObjectiveID       string
	ObjectiveStatus   string
	ObjectiveRevision uint64
	Goal              string
	Profile           string
	Acceptance        []string
	Done              []string
	Next              []string
	Validation        []string
	Blockers          []string
	MustRead          []string
	ResumeCommand     string
	BuiltAt           time.Time
}

type Source interface {
	Project(ctx context.Context) (projectID string, found bool, err error)
	Snapshot(ctx context.Context, projectID string) (State, Cursor, error)
	Head(ctx context.Context, projectID string) (Cursor, error)
}

type PublishStatus string

const (
	PublishCurrent PublishStatus = "current"
	PublishSkipped PublishStatus = "skipped"
)

type PublishResult struct {
	Status PublishStatus
	Cursor Cursor
}
