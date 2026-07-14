package workspace_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

func TestLegacyDirectHandoffResumesThroughEngineWithoutCreatingLedger(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "context.md"), "# context\n")
	mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), `---
{
  "blockers": [],
  "done": [
    "已定位根因"
  ],
  "engine": "direct",
  "goal": "继续修复登录回归",
  "last_writer": "session_fixture",
  "mode": "direct",
  "must_read": [
    "docs/context.md"
  ],
  "next": [
    "补充回归测试"
  ],
  "resume_command": "",
  "schema": "summer-harness/v1",
  "source_digest": "",
  "source_path": "",
  "task_id": "",
  "task_status": "",
  "updated_at": "2026-07-15T00:00:00.000000Z",
  "validation": []
}
---
# Project Handoff

## 当前目标

- 继续修复登录回归

## 已完成

- 已定位根因

## 唯一下一步

- 补充回归测试

## 必须读取

- docs/context.md
`)

	kernel, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	view, err := kernel.Query(context.Background(), engine.Query{Kind: engine.QueryResume})
	if err != nil {
		t.Fatalf("query resume: %v", err)
	}
	resume, ok := view.(engine.ResumeView)
	if !ok {
		t.Fatalf("view type = %T, want engine.ResumeView", view)
	}
	if resume.QueryKind() != engine.QueryResume {
		t.Fatalf("query kind = %q, want %q", resume.QueryKind(), engine.QueryResume)
	}
	if resume.Capsule.Schema != "summer-harness/v1" || resume.Capsule.Mode != "direct" {
		t.Fatalf("capsule identity = %#v", resume.Capsule)
	}
	if resume.Capsule.Goal != "继续修复登录回归" {
		t.Fatalf("goal = %q", resume.Capsule.Goal)
	}
	if !reflect.DeepEqual(resume.Capsule.Next, []string{"补充回归测试"}) {
		t.Fatalf("next = %#v", resume.Capsule.Next)
	}
	if !reflect.DeepEqual(resume.Capsule.MustRead, []string{"docs/context.md"}) {
		t.Fatalf("must_read = %#v", resume.Capsule.MustRead)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "ledger")); !os.IsNotExist(err) {
		t.Fatalf("resume created a ledger: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent", "runtime")); !os.IsNotExist(err) {
		t.Fatalf("resume created runtime state: %v", err)
	}
}

func TestLegacyNativeHandoffResumesCanonicalTaskThroughEngine(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# fixture\n")
	writeLegacyMemoryFixture(t, root, "task_20260714T184904300567Z_e160a8")
	mustWrite(t, filepath.Join(root, ".agent", "ledger", "tasks", "task_20260714T184904300567Z_e160a8.md"), `---
{
  "acceptance": [
    "恢复字段完整"
  ],
  "blockers": [],
  "created_at": "2026-07-14T18:49:04.300597Z",
  "created_by": "session_59bd844fa73bdd89",
  "done": [
    "完成契约"
  ],
  "engine": "summer",
  "goal": "跨 session 不丢上下文",
  "id": "task_20260714T184904300567Z_e160a8",
  "kind": "task",
  "last_work_session": "session_59bd844fa73bdd89",
  "last_writer": "session_59bd844fa73bdd89",
  "must_read": [
    "README.md"
  ],
  "next": [
    "实现 Go resume"
  ],
  "profile": "standard",
  "residual_risks": [],
  "review": {
    "approved": false,
    "findings": [],
    "reviewed_revision": 0,
    "summary": ""
  },
  "revision": 2,
  "risk": "medium",
  "schema": "summer-harness/v1",
  "status": "active",
  "title": "实现恢复",
  "updated_at": "2026-07-14T18:49:04.404534Z",
  "validation": [
    "fixture pass"
  ],
  "validation_revision": 2
}
---
# 实现恢复

## 目标

- 跨 session 不丢上下文

## 验收条件

- 恢复字段完整

## 已完成

- 完成契约

## 下一步

- 实现 Go resume

## 验证

- fixture pass

## 必须读取

- README.md
`)
	mustWrite(t, filepath.Join(root, ".agent", "HANDOFF.md"), `---
{
  "blockers": [],
  "done": [
    "完成契约"
  ],
  "engine": "summer",
  "goal": "跨 session 不丢上下文",
  "last_writer": "session_59bd844fa73bdd89",
  "mode": "native",
  "must_read": [
    "README.md"
  ],
  "next": [
    "实现 Go resume"
  ],
  "resume_command": "$project-handoff",
  "schema": "summer-harness/v1",
  "source_digest": "74f981d0c8c334fdd1ac16a9d8efdd29dd0d97eb87937b5393c0989072c2410f",
  "source_path": ".agent/ledger/tasks/task_20260714T184904300567Z_e160a8.md",
  "task_id": "task_20260714T184904300567Z_e160a8",
  "task_status": "active",
  "updated_at": "2026-07-14T18:49:04.404597Z",
  "validation": [
    "fixture pass"
  ]
}
---
# Project Handoff

## 当前目标

- 跨 session 不丢上下文

## 已完成

- 完成契约

## 唯一下一步

- 实现 Go resume

## 验证

- fixture pass

## 必须读取

- README.md
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
	if resume.Capsule.TaskID != "task_20260714T184904300567Z_e160a8" {
		t.Fatalf("task id = %q", resume.Capsule.TaskID)
	}
	if resume.Capsule.Status != "active" || resume.Capsule.Profile != "standard" || resume.Capsule.Risk != "medium" {
		t.Fatalf("task state = %#v", resume.Capsule)
	}
	if resume.Capsule.Revision != 2 {
		t.Fatalf("revision = %d", resume.Capsule.Revision)
	}
	if !reflect.DeepEqual(resume.Capsule.Acceptance, []string{"恢复字段完整"}) {
		t.Fatalf("acceptance = %#v", resume.Capsule.Acceptance)
	}
	if resume.Capsule.Decisions == nil || len(*resume.Capsule.Decisions) != 1 {
		t.Fatalf("decisions = %#v", resume.Capsule.Decisions)
	}
	if resume.Capsule.Facts == nil || len(*resume.Capsule.Facts) != 1 {
		t.Fatalf("facts = %#v", resume.Capsule.Facts)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
