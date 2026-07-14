package workspace_test

import (
	"path/filepath"
	"testing"
)

func writeLegacyMemoryFixture(t *testing.T, root, taskID string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, ".agent", "ledger", "decisions", "dec_fixture.md"), `---
{
  "chosen": "Handoff 只做投影",
  "created_at": "2026-07-15T00:01:00.000000Z",
  "created_by": "session_fixture",
  "created_ns": 1784073660000000000,
  "id": "dec_fixture",
  "kind": "decision",
  "question": "跨 session 状态放在哪里？",
  "rejected": [
    "聊天记录"
  ],
  "schema": "summer-harness/v1",
  "source": "fixture",
  "task_id": "task_20260714T184904300567Z_e160a8",
  "title": "唯一 Handoff",
  "why_not": [
    "聊天会被压缩"
  ]
}
---
# 唯一 Handoff

## 问题

- 跨 session 状态放在哪里？

## 选择

- Handoff 只做投影

## 拒绝

- 聊天记录

## 为什么不选

- 聊天会被压缩

## 来源

- fixture
`)
	mustWrite(t, filepath.Join(root, ".agent", "ledger", "facts", taskID+".jsonl"), `{"confidence":"high","created_ns":1784073720000000000,"id":"fact_fixture","kind":"fact","memory_class":"durable","observed_at":"2026-07-15T00:02:00.000000Z","schema":"summer-harness/v1","session":"session_fixture","source":"fixture","statement":"恢复测试通过","tags":["continuity"],"task_id":"task_20260714T184904300567Z_e160a8"}
`)
}
