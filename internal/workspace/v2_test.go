package workspace_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

func TestV2ObjectiveProjectsAndResumesThroughEngine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	payload, err := json.Marshal(engine.StartObjective{
		Title:      "实现 Continuity Kernel",
		Goal:       "跨 session 恢复不依赖聊天记录",
		Acceptance: []string{"Handoff 与 Ledger cursor 一致"},
		Profile:    "high-risk",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	receipt, err := kernel.Apply(context.Background(), engine.CommandEnvelope{
		Schema:           engine.CommandSchemaV2,
		CommandID:        "cmd-v2-resume",
		IdempotencyKey:   "start-v2-resume",
		CorrelationID:    "corr-v2-resume",
		ProjectID:        "project-v2-resume",
		ExpectedRevision: 0,
		IssuedAt:         time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC),
		Actor: engine.ActorRef{
			ActorID: "user-fixture", SessionID: "session-fixture", Runtime: "go-test", Role: engine.ActorUser,
		},
		Kind:    engine.CommandStartObjective,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("apply objective: %v", err)
	}
	if !receipt.Accepted {
		t.Fatalf("receipt rejected: %#v", receipt.Rejection)
	}
	if receipt.Projection == nil || receipt.Projection.Status != engine.ProjectionCurrent {
		t.Fatalf("projection receipt = %#v", receipt.Projection)
	}

	handoff := filepath.Join(root, ".agent", "HANDOFF.md")
	info, err := os.Stat(handoff)
	if err != nil {
		t.Fatalf("stat handoff: %v", err)
	}
	if info.Size() > continuity.HandoffLimit {
		t.Fatalf("handoff size = %d", info.Size())
	}

	restarted, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("reopen workspace: %v", err)
	}
	view, err := restarted.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("query resume: %v", err)
	}
	resume := view.(engine.ResumeView)
	if resume.Capsule.Schema != continuity.CapsuleSchemaV2 || resume.Capsule.Mode != continuity.ModeNative {
		t.Fatalf("capsule identity = %#v", resume.Capsule)
	}
	if resume.Capsule.ProjectID != "project-v2-resume" || resume.Capsule.ObjectiveID != receipt.EntityID {
		t.Fatalf("capsule binding = %#v", resume.Capsule)
	}
	if resume.Capsule.LedgerRevision != receipt.NewRevision || resume.Capsule.LedgerHead == "" {
		t.Fatalf("capsule cursor = %#v", resume.Capsule)
	}
	if resume.Capsule.Goal != "跨 session 恢复不依赖聊天记录" || resume.Capsule.Status != "active" {
		t.Fatalf("capsule state = %#v", resume.Capsule)
	}
	if resume.Capsule.Profile != "high-risk" || len(resume.Capsule.Acceptance) != 1 {
		t.Fatalf("capsule objective context = %#v", resume.Capsule)
	}
}
