package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestStartCreatesNativeObjectiveThatAWorkingCopyCanResume(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	var startOut, startErr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"start", "交付登录功能", "--repo", root, "--json"}, root, &startOut, &startErr)
	if exit != 0 || startErr.Len() != 0 {
		t.Fatalf("start exit = %d, stdout = %s, stderr = %s", exit, startOut.String(), startErr.String())
	}
	var started struct {
		Schema         string `json:"schema"`
		OK             bool   `json:"ok"`
		Command        string `json:"command"`
		ProjectID      string `json:"project_id"`
		ObjectiveID    string `json:"objective_id"`
		Status         string `json:"status"`
		LedgerRevision uint64 `json:"ledger_revision"`
		TransactionID  string `json:"transaction_id"`
		Projection     string `json:"projection"`
	}
	if err := json.Unmarshal(startOut.Bytes(), &started); err != nil {
		t.Fatalf("decode start result: %v\n%s", err, startOut.String())
	}
	if started.Schema != "summer.cli-result/v1" || !started.OK || started.Command != "start" || started.ProjectID == "" || started.ObjectiveID == "" || started.Status != "active" || started.LedgerRevision != 1 || started.TransactionID == "" || started.Projection != "current" {
		t.Fatalf("start result = %#v", started)
	}

	var resumeOut, resumeErr bytes.Buffer
	exit = cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &resumeOut, &resumeErr)
	if exit != 0 || resumeErr.Len() != 0 {
		t.Fatalf("resume exit = %d, stdout = %s, stderr = %s", exit, resumeOut.String(), resumeErr.String())
	}
	var capsule struct {
		Schema         string   `json:"schema"`
		Mode           string   `json:"mode"`
		Goal           string   `json:"goal"`
		Acceptance     []string `json:"acceptance"`
		Next           []string `json:"next"`
		ProjectID      string   `json:"project_id"`
		ObjectiveID    string   `json:"objective_id"`
		LedgerRevision uint64   `json:"ledger_revision"`
	}
	if err := json.Unmarshal(resumeOut.Bytes(), &capsule); err != nil {
		t.Fatalf("decode resume capsule: %v\n%s", err, resumeOut.String())
	}
	if capsule.Schema != continuity.CapsuleSchemaV2 || capsule.Mode != "native" || capsule.Goal != "交付登录功能" || !reflect.DeepEqual(capsule.Acceptance, []string{"交付登录功能"}) || !reflect.DeepEqual(capsule.Next, []string{"交付登录功能"}) || capsule.ProjectID != started.ProjectID || capsule.ObjectiveID != started.ObjectiveID || capsule.LedgerRevision != 1 {
		t.Fatalf("resume capsule = %#v, start = %#v", capsule, started)
	}
}

func TestStartExplicitlyReplacesADirectHandoffWithNativeV2(t *testing.T) {
	t.Parallel()

	root := writeDirectCLIRepo(t)
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"start", "显式进入 Summer", "--repo", root, "--json"}, root, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 {
		t.Fatalf("start exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var started struct {
		OK         bool   `json:"ok"`
		Committed  bool   `json:"committed"`
		Projection string `json:"projection"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &started); err != nil {
		t.Fatalf("decode start result: %v", err)
	}
	if !started.OK || !started.Committed || started.Projection != "current" {
		t.Fatalf("start result=%#v", started)
	}
	var resumed bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &resumed, io.Discard); exit != 0 {
		t.Fatalf("resume exit=%d output=%s", exit, resumed.String())
	}
	var capsule struct {
		Schema string `json:"schema"`
		Mode   string `json:"mode"`
		Goal   string `json:"goal"`
	}
	if err := json.Unmarshal(resumed.Bytes(), &capsule); err != nil {
		t.Fatalf("decode capsule: %v", err)
	}
	if capsule.Schema != continuity.CapsuleSchemaV2 || capsule.Mode != "native" || capsule.Goal != "显式进入 Summer" {
		t.Fatalf("capsule=%#v", capsule)
	}
}

func TestStartReportsCommittedProjectionRepairAsPartialFailure(t *testing.T) {
	t.Parallel()

	newBrokenCacheRepo := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".agent"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(root, ".agent", "cache")); err != nil {
			t.Fatal(err)
		}
		return root
	}

	root := newBrokenCacheRepo(t)
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"start", "验证部分成功", "--repo", root, "--json"}, root, &stdout, &stderr)
	if exit != 3 || stderr.Len() != 0 {
		t.Fatalf("start exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var result struct {
		OK         bool   `json:"ok"`
		Committed  bool   `json:"committed"`
		Projection string `json:"projection"`
		Code       string `json:"code"`
		Warning    string `json:"warning"`
		Hint       string `json:"hint"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.OK || !result.Committed || result.Projection != "repair_required" || result.Code == "" || result.Warning == "" || result.Hint == "" {
		t.Fatalf("partial result=%#v", result)
	}

	root = newBrokenCacheRepo(t)
	stdout.Reset()
	stderr.Reset()
	exit = cli.Run(context.Background(), []string{"start", "验证人类提示", "--repo", root}, root, &stdout, &stderr)
	if exit != 3 || !strings.Contains(stdout.String(), "警告:") || !strings.Contains(stdout.String(), "修复:") {
		t.Fatalf("human output exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
}

func TestStartAcceptsExplicitNextInsteadOfTheGoalDefault(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	if exit := cli.Run(context.Background(), []string{"start", "交付登录功能", "--next", "先确认会话边界", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("start exit = %d", exit)
	}
	var resumeOut bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &resumeOut, io.Discard); exit != 0 {
		t.Fatalf("resume exit = %d", exit)
	}
	var capsule struct {
		Next []string `json:"next"`
	}
	if err := json.Unmarshal(resumeOut.Bytes(), &capsule); err != nil {
		t.Fatalf("decode resume capsule: %v", err)
	}
	if !reflect.DeepEqual(capsule.Next, []string{"先确认会话边界"}) {
		t.Fatalf("next = %#v", capsule.Next)
	}
}

func TestStartRejectsMoreThanThreeNextItems(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{
		"start", "交付登录功能", "--repo", root, "--json",
		"--next", "一", "--next", "二", "--next", "三", "--next", "四",
	}, root, &stdout, &stderr)
	if exit != 2 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["code"] != "INVALID_COMMAND" {
		t.Fatalf("result=%#v", result)
	}
}

func TestSavePersistsAnActionableCheckpointAcrossCLIInstances(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	writeFile(t, filepath.Join(root, "docs", "auth.md"), []byte("# Auth\n"))
	if exit := cli.Run(context.Background(), []string{"start", "交付登录功能", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("start exit = %d", exit)
	}

	var saveOut, saveErr bytes.Buffer
	exit := cli.Run(context.Background(), []string{
		"save", "--repo", root, "--json",
		"--done", "完成会话模型",
		"--next", "实现登录端点",
		"--validation", "go test ./internal/session 通过",
		"--must-read", "docs/auth.md",
	}, root, &saveOut, &saveErr)
	if exit != 0 || saveErr.Len() != 0 {
		t.Fatalf("save exit = %d, stdout = %s, stderr = %s", exit, saveOut.String(), saveErr.String())
	}
	var saved struct {
		Schema            string `json:"schema"`
		OK                bool   `json:"ok"`
		Command           string `json:"command"`
		Status            string `json:"status"`
		ObjectiveRevision uint64 `json:"objective_revision"`
		LedgerRevision    uint64 `json:"ledger_revision"`
		Projection        string `json:"projection"`
	}
	if err := json.Unmarshal(saveOut.Bytes(), &saved); err != nil {
		t.Fatalf("decode save result: %v\n%s", err, saveOut.String())
	}
	if saved.Schema != "summer.cli-result/v1" || !saved.OK || saved.Command != "save" || saved.Status != "active" || saved.ObjectiveRevision != 2 || saved.LedgerRevision != 2 || saved.Projection != "current" {
		t.Fatalf("save result = %#v", saved)
	}

	var resumeOut, resumeErr bytes.Buffer
	exit = cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &resumeOut, &resumeErr)
	if exit != 0 || resumeErr.Len() != 0 {
		t.Fatalf("resume exit = %d, stdout = %s, stderr = %s", exit, resumeOut.String(), resumeErr.String())
	}
	var capsule struct {
		Done           []string `json:"done"`
		Next           []string `json:"next"`
		Validation     []string `json:"validation"`
		MustRead       []string `json:"must_read"`
		Revision       uint64   `json:"revision"`
		LedgerRevision uint64   `json:"ledger_revision"`
	}
	if err := json.Unmarshal(resumeOut.Bytes(), &capsule); err != nil {
		t.Fatalf("decode resume capsule: %v\n%s", err, resumeOut.String())
	}
	if !reflect.DeepEqual(capsule.Done, []string{"完成会话模型"}) || !reflect.DeepEqual(capsule.Next, []string{"实现登录端点"}) || !reflect.DeepEqual(capsule.Validation, []string{"go test ./internal/session 通过"}) || !reflect.DeepEqual(capsule.MustRead, []string{"docs/auth.md"}) || capsule.Revision != 2 || capsule.LedgerRevision != 2 {
		t.Fatalf("resume capsule = %#v", capsule)
	}
}

func TestStartRejectsASecondActiveObjectiveWithoutCreatingAnotherLifecycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	if exit := cli.Run(context.Background(), []string{"start", "第一个目标", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("first start exit = %d", exit)
	}
	var stdout, stderr bytes.Buffer
	exit := cli.Run(context.Background(), []string{"start", "第二个目标", "--repo", root, "--json"}, root, &stdout, &stderr)
	if exit != 2 || stderr.Len() != 0 {
		t.Fatalf("second start exit = %d, stdout = %s, stderr = %s", exit, stdout.String(), stderr.String())
	}
	var rejected struct {
		OK   bool   `json:"ok"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rejected); err != nil {
		t.Fatalf("decode rejection: %v\n%s", err, stdout.String())
	}
	if rejected.OK || rejected.Code != "OBJECTIVE_EXISTS" {
		t.Fatalf("rejection = %#v", rejected)
	}

	var resumeOut bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &resumeOut, io.Discard); exit != 0 {
		t.Fatalf("resume exit = %d", exit)
	}
	var capsule struct {
		Goal           string `json:"goal"`
		LedgerRevision uint64 `json:"ledger_revision"`
	}
	if err := json.Unmarshal(resumeOut.Bytes(), &capsule); err != nil {
		t.Fatalf("decode resume: %v", err)
	}
	if capsule.Goal != "第一个目标" || capsule.LedgerRevision != 1 {
		t.Fatalf("capsule = %#v", capsule)
	}
}

func TestStartKeepsHarnessIgnoreRulesInsideAgentDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	writeFile(t, filepath.Join(root, ".gitignore"), []byte("node_modules/\n"))
	if exit := cli.Run(context.Background(), []string{"start", "建立可恢复状态", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("start exit = %d", exit)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}
	want := "node_modules/\n"
	if string(raw) != want {
		t.Fatalf("gitignore = %q, want %q", raw, want)
	}
	agentRaw, err := os.ReadFile(filepath.Join(root, ".agent", ".gitignore"))
	if err != nil {
		t.Fatalf("read .agent/.gitignore: %v", err)
	}
	if string(agentRaw) != "runtime/\ncache/\n" {
		t.Fatalf(".agent/.gitignore = %q", agentRaw)
	}
}

func TestSaveCanBlockAndResumeTheCurrentObjective(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	if exit := cli.Run(context.Background(), []string{"start", "完成迁移", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("start exit = %d", exit)
	}
	if exit := cli.Run(context.Background(), []string{"save", "--blocker", "等待确认", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("block save exit = %d", exit)
	}
	var blockedOut bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &blockedOut, io.Discard); exit != 0 {
		t.Fatalf("blocked resume exit = %d", exit)
	}
	var blocked struct {
		Status   string   `json:"status"`
		Blockers []string `json:"blockers"`
	}
	if err := json.Unmarshal(blockedOut.Bytes(), &blocked); err != nil {
		t.Fatalf("decode blocked capsule: %v", err)
	}
	if blocked.Status != "blocked" || !reflect.DeepEqual(blocked.Blockers, []string{"等待确认"}) {
		t.Fatalf("blocked capsule = %#v", blocked)
	}

	if exit := cli.Run(context.Background(), []string{"save", "--clear-blockers", "--next", "继续导入", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("resume save exit = %d", exit)
	}
	var activeOut bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &activeOut, io.Discard); exit != 0 {
		t.Fatalf("active resume exit = %d", exit)
	}
	var active struct {
		Status   string   `json:"status"`
		Blockers []string `json:"blockers"`
		Next     []string `json:"next"`
	}
	if err := json.Unmarshal(activeOut.Bytes(), &active); err != nil {
		t.Fatalf("decode active capsule: %v", err)
	}
	if active.Status != "active" || len(active.Blockers) != 0 || !reflect.DeepEqual(active.Next, []string{"继续导入"}) {
		t.Fatalf("active capsule = %#v", active)
	}
}

func TestSaveCanReplaceBoundedDoneAndValidationSummaries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("create git directory: %v", err)
	}
	if exit := cli.Run(context.Background(), []string{"start", "长期项目", "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("start exit = %d", exit)
	}
	for index := 0; index < 8; index++ {
		value := fmt.Sprintf("进展-%d", index)
		validation := fmt.Sprintf("验证-%d", index)
		if exit := cli.Run(context.Background(), []string{"save", "--done", value, "--validation", validation, "--repo", root, "--json"}, root, io.Discard, io.Discard); exit != 0 {
			t.Fatalf("save %d exit = %d", index, exit)
		}
	}
	if exit := cli.Run(context.Background(), []string{
		"save", "--replace-done", "--done", "M1 已完成并压缩",
		"--replace-validation", "--validation", "全量验证通过", "--repo", root, "--json",
	}, root, io.Discard, io.Discard); exit != 0 {
		t.Fatalf("replace save exit = %d", exit)
	}
	var output bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &output, io.Discard); exit != 0 {
		t.Fatalf("resume exit = %d", exit)
	}
	var capsule struct {
		Done       []string `json:"done"`
		Validation []string `json:"validation"`
	}
	if err := json.Unmarshal(output.Bytes(), &capsule); err != nil {
		t.Fatalf("decode resume: %v", err)
	}
	if !reflect.DeepEqual(capsule.Done, []string{"M1 已完成并压缩"}) || !reflect.DeepEqual(capsule.Validation, []string{"全量验证通过"}) {
		t.Fatalf("capsule = %#v", capsule)
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
	if stdout.String() != "健康: 正常\n模式: direct\n" {
		t.Fatalf("doctor output = %q", stdout.String())
	}
}

func TestDefaultResumeOutputIsHumanAndJSONRemainsMachineStable(t *testing.T) {
	t.Parallel()

	root := writeDirectCLIRepo(t)
	var human bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root}, root, &human, io.Discard); exit != 0 {
		t.Fatalf("human resume exit = %d", exit)
	}
	if strings.HasPrefix(strings.TrimSpace(human.String()), "{") || !strings.Contains(human.String(), "目标: 继续修复") || !strings.Contains(human.String(), "下一步:") {
		t.Fatalf("human resume output = %q", human.String())
	}
	var machine bytes.Buffer
	if exit := cli.Run(context.Background(), []string{"resume", "--repo", root, "--json"}, root, &machine, io.Discard); exit != 0 {
		t.Fatalf("json resume exit = %d", exit)
	}
	var capsule map[string]any
	if err := json.Unmarshal(machine.Bytes(), &capsule); err != nil || capsule["mode"] != "direct" {
		t.Fatalf("json resume = %#v err=%v", capsule, err)
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

	for _, args := range [][]string{
		{"--json", "unknown"},
		{"resume", "--unknown", "--json"},
		{"--repo", "--json", "resume"},
		{"migrate", "--rollback", "--dry-run", "--json"},
		{"resume", "--rollback", "--json"},
		{"save", "--dry-run", "--json"},
		{"start", "目标", "--done", "不允许", "--json"},
	} {
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
