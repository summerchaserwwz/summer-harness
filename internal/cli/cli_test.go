package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/summerchaserwwz/summer-harness/internal/cli"
	"github.com/summerchaserwwz/summer-harness/internal/continuity"
)

func TestResumeAcceptsFlagsBeforeOrAfterSubcommand(t *testing.T) {
	t.Parallel()

	root := writeDirectCLIRepo(t)
	nested := filepath.Join(root, "src", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	cases := []struct {
		name string
		args []string
		cwd  string
	}{
		{name: "global flags", args: []string{"--repo", root, "--json", "resume"}, cwd: root},
		{name: "subcommand flags", args: []string{"resume", "--repo", root, "--json"}, cwd: root},
		{name: "discovery", args: []string{"resume", "--json"}, cwd: nested},
	}
	var first map[string]any
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exit := cli.Run(context.Background(), test.args, test.cwd, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("exit = %d, stderr = %s, stdout = %s", exit, stderr.String(), stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
			var capsule map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &capsule); err != nil {
				t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
			}
			if capsule["schema"] != "summer-harness/v1" || capsule["mode"] != "direct" {
				t.Fatalf("capsule = %#v", capsule)
			}
			if first == nil {
				first = capsule
			} else if !reflect.DeepEqual(first, capsule) {
				t.Fatalf("capsule differs: %#v != %#v", first, capsule)
			}
		})
	}
}

func TestResumeJSONErrorHasStableCodeAndExit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".agent", "HANDOFF.md"), bytes.Repeat([]byte("x"), continuity.HandoffLimit+1))
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", exit, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var result struct {
		OK    bool   `json:"ok"`
		Code  string `json:"code"`
		Error string `json:"error"`
		Hint  string `json:"hint"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result.OK || result.Code != string(continuity.CodeHandoffTooLarge) || result.Error == "" || result.Hint == "" {
		t.Fatalf("error result = %#v", result)
	}
}

func TestDoctorReportsCoreHealthWithoutOptionalIntegrations(t *testing.T) {
	t.Parallel()

	root := writeDirectCLIRepo(t)
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"doctor", "--repo", root}, root, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", exit, stdout.String(), stderr.String())
	}
	var result struct {
		OK       bool     `json:"ok"`
		Mode     string   `json:"mode"`
		Issues   []any    `json:"issues"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode doctor: %v", err)
	}
	if !result.OK || result.Mode != "direct" || len(result.Issues) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("doctor = %#v", result)
	}
}

func TestResumeP95StaysUnder100Milliseconds(t *testing.T) {
	root := writeDirectCLIRepo(t)
	durations := make([]time.Duration, 40)
	for index := range durations {
		started := time.Now()
		exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, io.Discard, io.Discard)
		durations[index] = time.Since(started)
		if exit != 0 {
			t.Fatalf("iteration %d exit = %d", index, exit)
		}
	}
	sort.Slice(durations, func(left, right int) bool { return durations[left] < durations[right] })
	p95 := durations[(len(durations)*95+99)/100-1]
	if p95 >= 100*time.Millisecond {
		t.Fatalf("resume p95 = %s, want <100ms", p95)
	}
}

func TestDiscoveryNeverCrossesNearestProjectBoundary(t *testing.T) {
	t.Parallel()

	parent := writeDirectCLIRepo(t)
	child := filepath.Join(parent, "child-project")
	if err := os.MkdirAll(filepath.Join(child, ".git"), 0o755); err != nil {
		t.Fatalf("create child project: %v", err)
	}
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"--json", "resume"}, child, &stdout, &stderr)
	if exit != 2 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", exit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["code"] != string(continuity.CodeHandoffNotFound) {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(stdout.String(), "继续修复") {
		t.Fatalf("child project resumed parent state: %s", stdout.String())
	}
}

func TestDiscoveryResolvesSymlinkedWorkingDirectoryBeforeWalkingParents(t *testing.T) {
	t.Parallel()

	parent := writeDirectCLIRepo(t)
	realProject := t.TempDir()
	realNested := filepath.Join(realProject, "src")
	if err := os.MkdirAll(realNested, 0o755); err != nil {
		t.Fatalf("create real nested directory: %v", err)
	}
	logical := filepath.Join(parent, "linked-project")
	if err := os.Symlink(realProject, logical); err != nil {
		t.Fatalf("create project symlink: %v", err)
	}
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"--json", "resume"}, filepath.Join(logical, "src"), &stdout, &stderr)
	if exit != 2 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %s, stderr = %s", exit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if result["code"] != string(continuity.CodeHandoffNotFound) {
		t.Fatalf("symlinked cwd resumed unrelated parent: %#v", result)
	}
}

func TestJSONModeCoversArgumentErrors(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"--json", "unknown"}, {"resume", "--unknown", "--json"}, {"--repo", "--json", "resume"}} {
		var stdout, stderr bytes.Buffer
		exit := cli.Run(context.Background(), args, t.TempDir(), &stdout, &stderr)
		if exit != 2 || stderr.Len() != 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, exit, stdout.String(), stderr.String())
		}
		var result map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("args=%v decode error: %v", args, err)
		}
		if result["code"] != "INVALID_ARGUMENT" {
			t.Fatalf("args=%v result=%#v", args, result)
		}
	}
}

func writeDirectCLIRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "context.md"), []byte("# context\n"))
	writeFile(t, filepath.Join(root, ".agent", "HANDOFF.md"), []byte(`---
{
  "blockers": [],
  "done": [
    "已定位原因"
  ],
  "engine": "direct",
  "goal": "继续修复",
  "last_writer": "session_fixture",
  "mode": "direct",
  "must_read": [
    "docs/context.md"
  ],
  "next": [
    "补测试"
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

- 继续修复

## 已完成

- 已定位原因

## 唯一下一步

- 补测试

## 必须读取

- docs/context.md
`))
	return root
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
