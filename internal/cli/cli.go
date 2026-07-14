package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/summerchaserwwz/summer-harness/internal/continuity"
	"github.com/summerchaserwwz/summer-harness/internal/engine"
	"github.com/summerchaserwwz/summer-harness/internal/workspace"
)

const Version = "0.1.0-dev"

type options struct {
	command string
	repo    string
	json    bool
	help    bool
	version bool
}

func Run(ctx context.Context, args []string, cwd string, stdout, stderr io.Writer) int {
	jsonIntent := false
	for _, argument := range args {
		if argument == "--json" {
			jsonIntent = true
			break
		}
	}
	options, err := parseArgs(args)
	if err != nil {
		return writeArgumentError(stdout, stderr, jsonIntent, err)
	}
	if options.version {
		fmt.Fprintf(stdout, "summer %s\n", Version)
		return 0
	}
	if options.help {
		fmt.Fprintln(stdout, usage())
		return 0
	}
	root, err := findRoot(options.repo, cwd)
	if err != nil {
		return writeError(stdout, stderr, options.json, continuity.CodeHandoffNotFound, err)
	}
	kernel, err := workspace.Open(root)
	if err != nil {
		return writeError(stdout, stderr, options.json, continuity.CodeHandoffInvalid, err)
	}
	switch options.command {
	case "resume":
		view, err := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume})
		if err != nil {
			code := continuity.ErrorCode(err)
			if code == "" {
				return writeInternalError(stdout, stderr, options.json, err)
			}
			return writeError(stdout, stderr, options.json, code, err)
		}
		resume, ok := view.(engine.ResumeView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
		}
		if err := writeJSON(stdout, resume.Capsule, !options.json); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		return 0
	case "doctor":
		view, queryErr := kernel.Query(ctx, engine.Query{Kind: engine.QueryResume})
		if queryErr != nil {
			code := continuity.ErrorCode(queryErr)
			if code == "" {
				code = "INTERNAL_ERROR"
			}
			result := map[string]any{
				"ok": false, "mode": "unknown", "issues": []map[string]string{{"code": string(code), "message": messageForCode(code)}},
				"warnings": []string{},
			}
			if err := writeJSON(stdout, result, !options.json); err != nil {
				return writeInternalError(stdout, stderr, options.json, err)
			}
			return 2
		}
		resume, ok := view.(engine.ResumeView)
		if !ok {
			return writeInternalError(stdout, stderr, options.json, fmt.Errorf("unexpected resume view %T", view))
		}
		result := map[string]any{
			"ok": true, "mode": resume.Capsule.Mode, "schema": resume.Capsule.Schema,
			"issues": []map[string]string{}, "warnings": []string{},
		}
		if err := writeJSON(stdout, result, !options.json); err != nil {
			return writeInternalError(stdout, stderr, options.json, err)
		}
		return 0
	default:
		fmt.Fprintln(stderr, "error: unsupported command")
		return 2
	}
}

func parseArgs(args []string) (options, error) {
	var result options
	commands := make([]string, 0, 1)
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--json":
			result.json = true
		case argument == "--help" || argument == "-h":
			result.help = true
		case argument == "--version":
			result.version = true
		case argument == "--repo":
			index++
			if index >= len(args) || strings.TrimSpace(args[index]) == "" || strings.HasPrefix(args[index], "-") {
				return options{}, errors.New("--repo requires a path")
			}
			result.repo = args[index]
		case strings.HasPrefix(argument, "--repo="):
			result.repo = strings.TrimPrefix(argument, "--repo=")
			if strings.TrimSpace(result.repo) == "" {
				return options{}, errors.New("--repo requires a path")
			}
		case strings.HasPrefix(argument, "-"):
			return options{}, fmt.Errorf("unknown flag %q", argument)
		default:
			commands = append(commands, argument)
		}
	}
	if result.version {
		if len(commands) > 0 {
			return options{}, errors.New("--version does not accept a command")
		}
		return result, nil
	}
	if result.help && len(commands) == 0 {
		return result, nil
	}
	if len(commands) != 1 || (commands[0] != "resume" && commands[0] != "doctor") {
		return options{}, errors.New("expected the resume or doctor command")
	}
	result.command = commands[0]
	return result, nil
}

func findRoot(explicit, cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(cwd, explicit)
		}
		absolute, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
			return "", fmt.Errorf("repository path is not a directory: %s", absolute)
		}
		return absolute, nil
	}
	current, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		return "", fmt.Errorf("current path is not a directory: %s", current)
	}
	for {
		handoff := filepath.Join(current, ".agent", "HANDOFF.md")
		if info, err := os.Lstat(handoff); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return current, nil
		}
		if _, err := os.Lstat(filepath.Join(current, ".agent")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("no .agent/HANDOFF.md found from the current directory")
		}
		current = parent
	}
}

func writeJSON(writer io.Writer, value any, indent bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if indent {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func writeError(stdout, stderr io.Writer, jsonOutput bool, code continuity.Code, cause error) int {
	message := messageForCode(code)
	hint := hintForCode(code)
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{"ok": false, "code": code, "error": message, "hint": hint}, false)
	} else {
		fmt.Fprintln(stderr, "error:", message)
		if hint != "" {
			fmt.Fprintln(stderr, "hint:", hint)
		}
	}
	_ = cause
	return 2
}

func writeInternalError(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{"ok": false, "code": "INTERNAL_ERROR", "error": "Summer Harness 内部错误", "hint": "运行 summer doctor 并保留错误输出"}, false)
	} else {
		fmt.Fprintln(stderr, "error: Summer Harness 内部错误")
		fmt.Fprintln(stderr, "detail:", cause)
	}
	return 1
}

func writeArgumentError(stdout, stderr io.Writer, jsonOutput bool, cause error) int {
	if jsonOutput {
		_ = writeJSON(stdout, map[string]any{
			"ok": false, "code": "INVALID_ARGUMENT", "error": cause.Error(), "hint": shortUsage(),
		}, false)
	} else {
		fmt.Fprintln(stderr, "error:", cause)
		fmt.Fprintln(stderr, shortUsage())
	}
	return 2
}

func messageForCode(code continuity.Code) string {
	switch code {
	case continuity.CodeHandoffNotFound:
		return "未找到 .agent/HANDOFF.md"
	case continuity.CodeHandoffTooLarge:
		return "Handoff 超过 4 KiB 限制"
	case continuity.CodeHandoffInvalid:
		return "Handoff 格式或内容无效"
	case continuity.CodeHandoffUnsupportedSchema:
		return "Handoff schema 不受支持"
	case continuity.CodeHandoffDrift:
		return "Handoff 与权威状态不一致"
	case continuity.CodeUnsafeReference:
		return "Handoff 包含不安全的文件引用"
	case continuity.CodeGSDPointerStale:
		return "GSD 状态指针已过期"
	case continuity.CodeCapsuleTooLarge:
		return "恢复胶囊超过 32 KiB 限制"
	case continuity.CodeProjectionStale, continuity.CodeProjectionConflict:
		return "Handoff 投影需要修复"
	case continuity.CodeCapabilityUnavailable:
		return "当前安装不支持该恢复能力"
	case continuity.CodeMigrationRequired:
		return "当前项目存在旧版生命周期状态，需要显式迁移"
	case continuity.CodeLifecycleConflict:
		return "当前项目存在冲突的生命周期所有者"
	default:
		return "Summer Harness 状态错误"
	}
}

func hintForCode(code continuity.Code) string {
	switch code {
	case continuity.CodeHandoffDrift, continuity.CodeProjectionStale, continuity.CodeProjectionConflict:
		return "运行 summer doctor；当前开发版请通过已安装的 $project-handoff 显式重建投影"
	case continuity.CodeHandoffNotFound:
		return "确认项目路径，或先使用 Summer Harness 保存交接"
	case continuity.CodeMigrationRequired:
		return "保留现有状态并运行显式迁移；不要直接覆盖 Handoff 或 Ledger"
	case continuity.CodeLifecycleConflict:
		return "运行 summer doctor，确认项目只能有一个生命周期所有者"
	default:
		return "运行 summer doctor 获取诊断结果"
	}
}

func shortUsage() string {
	return "usage: summer [--repo <path>] [--json] <resume|doctor>"
}

func usage() string {
	return `Summer Harness

Usage:
  summer [--repo <path>] [--json] resume
  summer resume [--repo <path>] [--json]
  summer [--repo <path>] [--json] doctor
  summer doctor [--repo <path>] [--json]
  summer --version`
}
