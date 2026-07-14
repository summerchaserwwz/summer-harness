package workspace_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

func TestLegacyGSDHandoffResumesDigestBoundPointer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".planning", "STATE.md"), "# state\nphase: 2\n")
	mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), `---
{
  "blockers": [],
  "done": [],
  "engine": "gsd",
  "goal": "交付多阶段项目",
  "last_writer": "session_59bd844fa73bdd89",
  "mode": "gsd",
  "must_read": [],
  "next": [
    "恢复第二阶段"
  ],
  "resume_command": "$gsd-resume-work",
  "schema": "summer-harness/v1",
  "source_digest": "db7dfc6be87024a42908b94b9358735bc1a9e3508afdef1de72107f33c95044e",
  "source_path": ".planning/STATE.md",
  "task_id": "",
  "task_status": "",
  "updated_at": "2026-07-14T18:53:23.278973Z",
  "validation": []
}
---
# Project Handoff

## 当前目标

- 交付多阶段项目

## 唯一下一步

- 恢复第二阶段
`)

	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("query resume: %v", err)
	}
	resume := view.(engine.ResumeView)
	if resume.Capsule.Mode != "gsd" || resume.Capsule.Engine != "gsd" {
		t.Fatalf("capsule route = %#v", resume.Capsule)
	}
	if resume.Capsule.SourcePath != ".planning/STATE.md" || resume.Capsule.ResumeCommand != "$gsd-resume-work" {
		t.Fatalf("capsule pointer = %#v", resume.Capsule)
	}
}
