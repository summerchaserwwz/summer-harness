#!/usr/bin/env python3
"""Summer Harness: a small, file-only workflow kernel."""

from __future__ import annotations

import argparse
import contextlib
import datetime as dt
import hashlib
import json
import os
import pathlib
import re
import sys
import tempfile
import time
import uuid
from typing import Any, Dict, Iterable, Iterator, List, Optional, Sequence, Tuple


SCHEMA = "summer-harness/v1"
ACTIVE_STATES = {"active", "blocked", "review"}
PROFILES = {"standard", "high-risk", "release", "research"}
RISKS = {"low", "medium", "high"}
HANDOFF_LIMIT = 4096
CAPSULE_LIMIT = 32768
MAX_TEXT = 2000
MAX_PATH = 500
MAX_MUST_READ = 5
MAX_DONE = 8
MAX_NEXT = 3
MAX_VALIDATION = 8
MAX_BLOCKERS = 5


class HarnessError(RuntimeError):
    pass


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")


def session_id() -> str:
    raw = os.environ.get("CODEX_THREAD_ID") or os.environ.get("SUMMER_SESSION_ID")
    if not raw:
        return "session_unknown"
    return "session_" + hashlib.sha256(raw.encode("utf-8")).hexdigest()[:16]


def new_id(prefix: str) -> str:
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    return f"{prefix}_{stamp}_{uuid.uuid4().hex[:6]}"


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def json_dump(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True)


def atomic_write(path: pathlib.Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temp_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temp_name, path)
        fsync_directory(path.parent)
    finally:
        if os.path.exists(temp_name):
            os.unlink(temp_name)


def fsync_directory(path: pathlib.Path) -> None:
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    with contextlib.suppress(OSError):
        fd = os.open(str(path), flags)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)


def atomic_append(path: pathlib.Path, content: str) -> None:
    existing = path.read_text(encoding="utf-8") if path.exists() else ""
    atomic_write(path, existing + content)


def render_markdown(meta: Dict[str, Any], title: str, sections: Sequence[Tuple[str, Iterable[str]]]) -> str:
    lines = ["---", json_dump(meta), "---", f"# {title}", ""]
    for heading, values in sections:
        cleaned = [str(value).strip() for value in values if str(value).strip()]
        if not cleaned:
            continue
        lines.extend([f"## {heading}", ""])
        lines.extend(f"- {value}" for value in cleaned)
        lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def parse_markdown(path: pathlib.Path) -> Tuple[Dict[str, Any], str]:
    try:
        raw = path.read_text(encoding="utf-8")
    except FileNotFoundError as exc:
        raise HarnessError(f"缺少文件：{path}") from exc
    match = re.match(r"\A---\s*\n(.*?)\n---\s*\n", raw, re.DOTALL)
    if not match:
        raise HarnessError(f"frontmatter 无效：{path}")
    try:
        meta = json.loads(match.group(1))
    except json.JSONDecodeError as exc:
        raise HarnessError(f"frontmatter JSON 无效：{path}: {exc}") from exc
    if meta.get("schema") != SCHEMA:
        raise HarnessError(f"不支持的 schema：{path}")
    return meta, raw[match.end() :]


def clean_text(value: Optional[str], label: str, limit: int = MAX_TEXT, required: bool = False) -> str:
    cleaned = (value or "").strip()
    if required and not cleaned:
        raise HarnessError(f"{label} 不能为空")
    if len(cleaned) > limit:
        raise HarnessError(f"{label} 超过 {limit} 个字符")
    return cleaned


def bounded(values: Optional[Sequence[str]], limit: int, item_limit: int = MAX_TEXT, label: str = "字段") -> List[str]:
    if not values:
        return []
    cleaned = []
    for value in values:
        value = clean_text(value, label, item_limit)
        if value and value not in cleaned:
            cleaned.append(value)
    return cleaned[-limit:]


class Repo:
    def __init__(self, root: pathlib.Path):
        self.root = root.resolve()
        self.agent = self.root / ".agent"
        self.config = self.agent / "harness.json"
        self.handoff = self.agent / "HANDOFF.md"
        self.tasks = self.agent / "ledger" / "tasks"
        self.decisions = self.agent / "ledger" / "decisions"
        self.facts = self.agent / "ledger" / "facts"
        self.archive = self.agent / "archive"
        self.runtime = self.agent / "runtime"
        self.lock_path = self.runtime / "write.lock"
        self.transaction_path = self.runtime / "transaction.json"

    def require(self) -> None:
        if not self.config.exists():
            raise HarnessError("当前项目尚未初始化 Summer Harness；请先运行 init")

    def require_handoff(self) -> None:
        if not self.handoff.exists():
            raise HarnessError("当前项目没有 .agent/HANDOFF.md")

    def ensure_safe_state_tree(self) -> None:
        for path in (self.agent, self.agent / "ledger", self.tasks, self.decisions, self.facts,
                     self.archive, self.runtime):
            if path.is_symlink():
                raise HarnessError(f"Harness 状态路径不能是符号链接：{path}")
            if path.exists() and not path.is_dir():
                raise HarnessError(f"Harness 状态目录被非目录占用：{path}")
        for path in (self.config, self.handoff, self.lock_path, self.transaction_path):
            if path.is_symlink():
                raise HarnessError(f"Harness 状态文件不能是符号链接：{path}")
        for directory, pattern in ((self.tasks, "*.md"), (self.decisions, "*.md"), (self.facts, "*.jsonl")):
            if directory.exists():
                for path in directory.glob(pattern):
                    if path.is_symlink():
                        raise HarnessError(f"Harness 账本文件不能是符号链接：{path}")

    def validate_reference(self, value: str, label: str, required_prefix: Optional[str] = None) -> str:
        value = clean_text(value, label, MAX_PATH, required=True)
        relative = pathlib.Path(value)
        if relative.is_absolute() or ".." in relative.parts:
            raise HarnessError(f"{label} 必须是仓库内相对路径")
        if required_prefix and (not relative.parts or relative.parts[0] != required_prefix):
            raise HarnessError(f"{label} 必须位于 {required_prefix}/ 内")
        candidate = self.root / relative
        current = self.root
        for part in relative.parts:
            current = current / part
            if current.is_symlink():
                raise HarnessError(f"{label} 不能经过符号链接：{value}")
        if not candidate.is_file():
            raise HarnessError(f"{label} 不存在或不是文件：{value}")
        try:
            candidate.resolve().relative_to(self.root)
        except ValueError as exc:
            raise HarnessError(f"{label} 指向仓库外：{value}") from exc
        return relative.as_posix()

    def validate_must_read(self, values: Optional[Sequence[str]]) -> List[str]:
        cleaned = bounded(values, MAX_MUST_READ, MAX_PATH, "must_read")
        return [self.validate_reference(value, "must_read") for value in cleaned]

    def _pid_alive(self, pid: Any) -> bool:
        try:
            os.kill(int(pid), 0)
            return True
        except (ProcessLookupError, ValueError, TypeError):
            return False
        except PermissionError:
            return True

    def _read_lock(self) -> Dict[str, Any]:
        try:
            return json.loads(self.lock_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return {}

    @contextlib.contextmanager
    def lock(self) -> Iterator[None]:
        self.ensure_safe_state_tree()
        self.runtime.mkdir(parents=True, exist_ok=True)
        token = uuid.uuid4().hex
        payload = json.dumps({"pid": os.getpid(), "token": token, "created_at": utc_now()})
        try:
            fd = os.open(str(self.lock_path), os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        except FileExistsError:
            owner = self._read_lock()
            age = time.time() - self.lock_path.stat().st_mtime if self.lock_path.exists() else 0
            if self._pid_alive(owner.get("pid")) or age <= 600:
                raise HarnessError("存在另一个 Harness 写入者；请检查 .agent/runtime/write.lock")
            self.lock_path.unlink(missing_ok=True)
            fd = os.open(str(self.lock_path), os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
        try:
            os.write(fd, payload.encode("utf-8"))
            os.fsync(fd)
            os.close(fd)
            self.recover_transaction()
            yield
        finally:
            owner = self._read_lock()
            if owner.get("token") == token:
                with contextlib.suppress(FileNotFoundError):
                    self.lock_path.unlink()
            if not self.config.exists():
                with contextlib.suppress(OSError):
                    self.runtime.rmdir()

    def recover_transaction(self) -> None:
        if not self.transaction_path.exists():
            return
        try:
            journal = json.loads(self.transaction_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise HarnessError("事务日志损坏；停止写入并人工检查 .agent/runtime/transaction.json") from exc
        if journal.get("schema") != SCHEMA or not isinstance(journal.get("entries"), list):
            raise HarnessError("事务日志 schema 无效")
        for entry in journal["entries"]:
            relative = pathlib.Path(entry.get("path", ""))
            if relative.is_absolute() or ".." in relative.parts or not relative.parts or relative.parts[0] != ".agent":
                raise HarnessError("事务日志包含越界路径")
            allowed_task = (len(relative.parts) == 4 and relative.parts[:3] == (".agent", "ledger", "tasks")
                            and re.fullmatch(r"task_[A-Za-z0-9_\-]+\.md", relative.name))
            if relative.as_posix() != ".agent/HANDOFF.md" and not allowed_task:
                raise HarnessError("事务日志包含非 Task/Handoff 路径")
            target = self.root / relative
            if target.is_symlink() or target.parent.is_symlink():
                raise HarnessError("事务日志目标不能是符号链接")
            atomic_write(target, str(entry.get("content", "")))
        self.transaction_path.unlink()
        fsync_directory(self.runtime)

    def transactional_write(self, entries: Sequence[Tuple[pathlib.Path, str]]) -> None:
        journal_entries = []
        for path, content in entries:
            try:
                relative = path.relative_to(self.root)
            except ValueError as exc:
                raise HarnessError("事务写入路径越界") from exc
            journal_entries.append({"path": relative.as_posix(), "content": content})
        atomic_write(self.transaction_path, json_dump({
            "schema": SCHEMA, "transaction_id": uuid.uuid4().hex, "created_at": utc_now(),
            "entries": journal_entries,
        }) + "\n")
        self.recover_transaction()

    def task_path(self, task_id: str) -> pathlib.Path:
        if not re.fullmatch(r"task_[A-Za-z0-9_\-]+", task_id):
            raise HarnessError("task id 非法")
        return self.tasks / f"{task_id}.md"

    def decision_path(self, decision_id: str) -> pathlib.Path:
        return self.decisions / f"{decision_id}.md"

    def fact_path(self, task_id: str) -> pathlib.Path:
        return self.facts / f"{task_id}.jsonl"

    def read_handoff(self) -> Dict[str, Any]:
        return parse_markdown(self.handoff)[0]

    def read_task(self, task_id: str) -> Dict[str, Any]:
        return parse_markdown(self.task_path(task_id))[0]

    def render_task(self, task: Dict[str, Any]) -> Tuple[str, str]:
        task["updated_at"] = utc_now()
        task["last_writer"] = session_id()
        content = render_markdown(task, task["title"], [
            ("目标", [task["goal"]]),
            ("验收条件", task.get("acceptance", [])),
            ("已完成", task.get("done", [])),
            ("下一步", task.get("next", [])),
            ("验证", task.get("validation", [])),
            ("阻塞", task.get("blockers", [])),
            ("必须读取", task.get("must_read", [])),
            ("残余风险", task.get("residual_risks", [])),
        ])
        return content, sha256_bytes(content.encode("utf-8"))

    def render_handoff(self, handoff: Dict[str, Any]) -> Tuple[Dict[str, Any], str]:
        handoff = dict(handoff)
        handoff.update({"schema": SCHEMA, "updated_at": utc_now(), "last_writer": session_id()})
        for key, limit in (("done", MAX_DONE), ("next", MAX_NEXT), ("validation", MAX_VALIDATION),
                           ("blockers", MAX_BLOCKERS), ("must_read", MAX_MUST_READ)):
            handoff[key] = bounded(handoff.get(key), limit, MAX_PATH if key == "must_read" else MAX_TEXT, key)
        content = render_markdown(handoff, "Project Handoff", [
            ("当前目标", [handoff.get("goal", "")]),
            ("已完成", handoff["done"]),
            ("唯一下一步", handoff["next"]),
            ("验证", handoff["validation"]),
            ("阻塞", handoff["blockers"]),
            ("必须读取", handoff["must_read"]),
        ])
        if len(content.encode("utf-8")) > HANDOFF_LIMIT:
            raise HarnessError(f"HANDOFF 超过 {HANDOFF_LIMIT} 字节；请压缩描述或减少引用")
        return handoff, content

    def write_handoff(self, handoff: Dict[str, Any]) -> None:
        _, content = self.render_handoff(handoff)
        atomic_write(self.handoff, content)

    def native_handoff(self, task: Dict[str, Any], digest: str) -> Dict[str, Any]:
        return {
            "mode": "native", "engine": "summer", "task_id": task["id"],
            "task_status": task["status"],
            "source_path": str(self.task_path(task["id"]).relative_to(self.root)),
            "source_digest": digest, "goal": task["goal"], "done": task.get("done", []),
            "next": task.get("next", []), "validation": task.get("validation", []),
            "blockers": task.get("blockers", []), "must_read": task.get("must_read", []),
            "resume_command": "$project-handoff",
        }

    def commit_task(self, task: Dict[str, Any], handoff: Optional[Dict[str, Any]] = None) -> str:
        task_content, digest = self.render_task(task)
        if handoff is None:
            handoff = self.native_handoff(task, digest)
        else:
            handoff = dict(handoff)
            handoff["source_digest"] = digest
        _, handoff_content = self.render_handoff(handoff)
        self.transactional_write([
            (self.task_path(task["id"]), task_content),
            (self.handoff, handoff_content),
        ])
        return digest


def find_repo(explicit: Optional[str]) -> Repo:
    if explicit:
        return Repo(pathlib.Path(explicit))
    current = pathlib.Path.cwd().resolve()
    for candidate in [current] + list(current.parents):
        if (candidate / ".agent" / "HANDOFF.md").exists() or (candidate / ".agent" / "harness.json").exists():
            return Repo(candidate)
    return Repo(current)


def active_task(repo: Repo) -> Tuple[Dict[str, Any], Dict[str, Any]]:
    handoff = repo.read_handoff()
    if handoff.get("mode") != "native" or not handoff.get("task_id"):
        raise HarnessError("当前没有 Summer Harness 原生活跃任务")
    task_path = repo.task_path(handoff["task_id"])
    if sha256_bytes(task_path.read_bytes()) != handoff.get("source_digest"):
        raise HarnessError("HANDOFF 与 Task 摘要不一致；先运行 doctor，不要覆盖未知改动")
    task = repo.read_task(handoff["task_id"])
    if task.get("status") not in ACTIVE_STATES:
        raise HarnessError(f"任务不是活跃状态：{task.get('status')}")
    active_ids = []
    for path in repo.tasks.glob("task_*.md"):
        meta = parse_markdown(path)[0]
        if meta.get("status") in ACTIVE_STATES:
            active_ids.append(meta.get("id"))
    if active_ids != [task["id"]]:
        raise HarnessError("活跃 Task 与唯一 Handoff 不一致；运行 doctor")
    return task, handoff


def cmd_init(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    with repo.lock():
        for path in (repo.tasks, repo.decisions, repo.facts, repo.archive, repo.runtime):
            path.mkdir(parents=True, exist_ok=True)
        if not repo.config.exists():
            atomic_write(repo.config, json_dump({
                "schema": SCHEMA, "created_at": utc_now(), "handoff": ".agent/HANDOFF.md",
                "native_ledger": ".agent/ledger", "gsd_source": ".planning",
            }) + "\n")
        if not repo.handoff.exists():
            repo.write_handoff({
                "mode": "idle", "engine": "none", "goal": "", "done": [], "next": [],
                "validation": [], "blockers": [], "must_read": [], "resume_command": "",
            })
        gitignore = repo.root / ".gitignore"
        current = gitignore.read_text(encoding="utf-8") if gitignore.exists() else ""
        if ".agent/runtime/" not in current.splitlines():
            prefix = current + ("" if not current or current.endswith("\n") else "\n")
            atomic_write(gitignore, prefix + ".agent/runtime/\n")
    return {"ok": True, "root": str(repo.root), "handoff": str(repo.handoff)}


def cmd_start(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        handoff = repo.read_handoff()
        if handoff.get("mode") != "idle":
            raise HarnessError("已有非 idle Handoff；请先恢复或显式清空，避免覆盖另一生命周期")
        orphaned = [path for path in repo.tasks.glob("task_*.md") if parse_markdown(path)[0].get("status") in ACTIVE_STATES]
        if orphaned:
            raise HarnessError("检测到未被 Handoff 引用的活跃 Task；先运行 doctor")
        task = {
            "schema": SCHEMA, "kind": "task", "id": new_id("task"), "engine": "summer",
            "title": clean_text(args.title, "title", 200, True), "goal": clean_text(args.goal, "goal", MAX_TEXT, True),
            "status": "active", "profile": args.profile, "risk": args.risk,
            "acceptance": bounded(args.acceptance, 12, 1000, "acceptance"),
            "done": [], "next": bounded(args.next, MAX_NEXT, 1000, "next"), "validation": [], "blockers": [],
            "must_read": repo.validate_must_read(args.must_read), "residual_risks": [],
            "revision": 1, "validation_revision": 0,
            "review": {"approved": False, "summary": "", "findings": [], "reviewed_revision": 0},
            "created_at": utc_now(), "created_by": session_id(), "updated_at": utc_now(),
            "last_writer": session_id(), "last_work_session": session_id(),
        }
        if not task["title"] or not task["goal"] or not task["acceptance"]:
            raise HarnessError("start 必须提供 title、goal 和至少一个 acceptance")
        repo.commit_task(task)
    return {"ok": True, "task_id": task["id"], "status": task["status"]}


def merge_recent(existing: Sequence[str], incoming: Optional[Sequence[str]], limit: int) -> List[str]:
    return bounded(list(existing) + list(incoming or []), limit)


def cmd_checkpoint(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        task, _ = active_task(repo)
        changed = bool(args.done or args.next or args.validation or args.blocker or args.must_read or args.clear_blockers or args.replace_done)
        if not changed:
            raise HarnessError("checkpoint 至少需要一个更新字段")
        if args.replace_done and not args.done:
            raise HarnessError("--replace-done 必须同时提供至少一个 --done 摘要")
        task["revision"] = int(task.get("revision", 1)) + 1
        task["last_work_session"] = session_id()
        incoming_done = bounded(args.done, MAX_DONE, 1000, "done")
        task["done"] = incoming_done if args.replace_done else merge_recent(task.get("done", []), incoming_done, MAX_DONE)
        task["next"] = bounded(args.next, MAX_NEXT, 1000, "next") if args.next else task.get("next", [])
        incoming_validation = bounded(args.validation, MAX_VALIDATION, MAX_TEXT, "validation")
        task["validation"] = merge_recent(task.get("validation", []), incoming_validation, MAX_VALIDATION)
        if incoming_validation:
            task["validation_revision"] = task["revision"]
        task["must_read"] = repo.validate_must_read(args.must_read) if args.must_read else task.get("must_read", [])
        if args.blocker:
            task["blockers"], task["status"] = bounded(args.blocker, MAX_BLOCKERS, 1000, "blocker"), "blocked"
        elif args.clear_blockers:
            task["blockers"], task["status"] = [], "active"
        elif task.get("status") == "review":
            task["status"] = "active"
        task["review"] = {"approved": False, "summary": "", "findings": [], "reviewed_revision": 0}
        repo.commit_task(task)
    return {"ok": True, "task_id": task["id"], "status": task["status"]}


def cmd_fact(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        task, _ = active_task(repo)
        if args.invalidate:
            reason = clean_text(args.reason, "reason", 1000, True)
            record = {"schema": SCHEMA, "kind": "fact_invalidation", "id": new_id("inv"),
                      "invalidates": clean_text(args.invalidate, "Fact ID", 200, True), "reason": reason,
                      "task_id": task["id"], "observed_at": utc_now(), "created_ns": time.time_ns(),
                      "session": session_id()}
        else:
            statement = clean_text(args.statement, "statement", MAX_TEXT, True)
            source = clean_text(args.source, "source", 1000, True)
            record = {"schema": SCHEMA, "kind": "fact", "id": new_id("fact"), "task_id": task["id"],
                      "statement": statement, "source": source,
                      "confidence": args.confidence, "memory_class": args.memory_class,
                      "tags": bounded(args.tag, 8, 100, "tag"), "observed_at": utc_now(),
                      "created_ns": time.time_ns(), "session": session_id()}
        atomic_append(repo.fact_path(task["id"]), json.dumps(record, ensure_ascii=False, sort_keys=True) + "\n")
    return {"ok": True, "id": record["id"], "kind": record["kind"]}


def cmd_decision(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        task, _ = active_task(repo)
        decision = {"schema": SCHEMA, "kind": "decision", "id": new_id("dec"), "task_id": task["id"],
                    "title": clean_text(args.title, "title", 200, True),
                    "question": clean_text(args.question, "question", 1000, True),
                    "chosen": clean_text(args.chosen, "chosen", MAX_TEXT, True),
                    "rejected": bounded(args.rejected, 8, 1000, "rejected"),
                    "why_not": bounded(args.why_not, 8, 1000, "why_not"),
                    "source": clean_text(args.source, "source", 1000, True), "created_at": utc_now(),
                    "created_ns": time.time_ns(), "created_by": session_id()}
        content = render_markdown(decision, decision["title"], [
            ("问题", [decision["question"]]), ("选择", [decision["chosen"]]),
            ("拒绝", decision["rejected"]), ("为什么不选", decision["why_not"]),
            ("来源", [decision["source"]]),
        ])
        atomic_write(repo.decision_path(decision["id"]), content)
    return {"ok": True, "decision_id": decision["id"]}


def cmd_review(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        task, _ = active_task(repo)
        if args.approved and args.finding:
            raise HarnessError("存在 finding 时不能标记 approved")
        reviewer = clean_text(args.reviewer, "reviewer", 200, True)
        if reviewer == task.get("last_work_session"):
            raise HarnessError("reviewer 必须与最近工作 session 不同")
        if task.get("profile") in {"high-risk", "release"} and args.approved and not args.independent:
            raise HarnessError("高风险/发布审查必须声明 --independent")
        task["review"] = {"approved": bool(args.approved),
                          "summary": clean_text(args.summary, "summary", MAX_TEXT, True),
                          "findings": bounded(args.finding, 12, 1000, "finding"), "reviewed_at": utc_now(),
                          "reviewed_by": reviewer, "independent": bool(args.independent),
                          "reviewed_revision": int(task.get("revision", 1))}
        task["status"] = "review"
        repo.commit_task(task)
    return {"ok": True, "task_id": task["id"], "approved": bool(args.approved)}


def completion_issues(repo: Repo, task: Dict[str, Any], args: argparse.Namespace) -> List[str]:
    issues = []
    if not task.get("acceptance"):
        issues.append("缺少验收条件")
    incoming_validation = bounded(args.validation, MAX_VALIDATION, MAX_TEXT, "validation")
    if not merge_recent(task.get("validation", []), incoming_validation, MAX_VALIDATION):
        issues.append("缺少真实验证记录")
    effective_validation_revision = int(task.get("revision", 1)) if incoming_validation else int(task.get("validation_revision", 0))
    if effective_validation_revision != int(task.get("revision", 1)):
        issues.append("验证未覆盖当前任务修订")
    if task.get("blockers"):
        issues.append("仍有未解除阻塞")
    if task.get("status") == "review" and not task.get("review", {}).get("approved"):
        issues.append("当前审查尚未通过")
    review = task.get("review", {})
    if task.get("profile") in {"high-risk", "release"}:
        if not review.get("approved") or review.get("reviewed_revision") != task.get("revision"):
            issues.append("高风险/发布任务缺少覆盖当前修订的已通过审查")
        if not review.get("independent"):
            issues.append("高风险/发布任务缺少独立审查声明")
    if task.get("profile") == "research" and not read_facts(repo, task["id"]):
        issues.append("研究任务缺少带来源的有效 Fact")
    residuals = merge_recent(task.get("residual_risks", []), bounded(args.residual_risk, 8, 1000, "residual_risk"), 8)
    if task.get("profile") == "release" and residuals and not args.ack_residual:
        issues.append("发布任务的残余风险尚未显式确认")
    return issues


def cmd_complete(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        task, _ = active_task(repo)
        summary = clean_text(args.summary, "summary", MAX_TEXT, True)
        issues = completion_issues(repo, task, args)
        if issues:
            raise HarnessError("完成门禁未通过：" + "；".join(issues))
        incoming_validation = bounded(args.validation, MAX_VALIDATION, MAX_TEXT, "validation")
        task["validation"] = merge_recent(task.get("validation", []), incoming_validation, MAX_VALIDATION)
        if incoming_validation:
            task["validation_revision"] = task["revision"]
        task["residual_risks"] = merge_recent(task.get("residual_risks", []),
                                               bounded(args.residual_risk, 8, 1000, "residual_risk"), 8)
        task["status"] = "done"
        task["closeout"] = {"summary": summary, "completed_at": utc_now(),
                            "completed_by": session_id(), "residual_acknowledged": bool(args.ack_residual)}
        repo.commit_task(task, {
            "mode": "idle", "engine": "none", "task_id": task["id"], "task_status": "done",
            "source_path": str(repo.task_path(task["id"]).relative_to(repo.root)),
            "goal": task["goal"], "done": merge_recent(task.get("done", []), [summary], MAX_DONE),
            "next": [], "validation": task["validation"], "blockers": [], "must_read": [], "resume_command": "",
        })
    return {"ok": True, "task_id": task["id"], "status": "done"}


def cmd_handoff(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    goal = clean_text(args.goal, "goal", MAX_TEXT, required=args.mode != "idle")
    with repo.lock():
        current = repo.read_handoff() if repo.handoff.exists() else {"mode": "idle"}
        if current.get("mode") == "native" and current.get("task_status") in ACTIVE_STATES:
            raise HarnessError("活跃 Summer Task 只能通过 checkpoint/complete 更新交接")
        active_ids = [parse_markdown(path)[0].get("id") for path in repo.tasks.glob("task_*.md")
                      if parse_markdown(path)[0].get("status") in ACTIVE_STATES]
        if active_ids:
            raise HarnessError("存在未完成的 Summer Task，不能切换 Handoff 模式")
        must_read = repo.validate_must_read(args.must_read)
        source_path, source_digest = "", ""
        if args.mode == "gsd":
            source_path = repo.validate_reference(args.active_artifact or ".planning/STATE.md", "GSD active_artifact", ".planning")
            source_digest = sha256_bytes((repo.root / source_path).read_bytes())
        repo.write_handoff({
            "mode": args.mode, "engine": "gsd" if args.mode == "gsd" else ("direct" if args.mode == "direct" else "none"),
            "task_id": "", "task_status": "", "source_path": source_path,
            "source_digest": source_digest, "goal": goal,
            "done": bounded(args.done, MAX_DONE, 1000, "done"),
            "next": bounded(args.next, MAX_NEXT, 1000, "next"),
            "validation": bounded(args.validation, MAX_VALIDATION, MAX_TEXT, "validation"),
            "blockers": bounded(args.blocker, MAX_BLOCKERS, 1000, "blocker"), "must_read": must_read,
            "resume_command": clean_text(args.resume_command, "resume_command", 500) or ("$gsd-resume-work" if args.mode == "gsd" else ""),
        })
    return {"ok": True, "mode": args.mode, "handoff": str(repo.handoff)}


def cmd_repair_handoff(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require()
    with repo.lock():
        active = []
        for path in repo.tasks.glob("task_*.md"):
            meta = parse_markdown(path)[0]
            if meta.get("status") in ACTIVE_STATES:
                active.append(meta)
        if len(active) != 1:
            raise HarnessError("repair-handoff 要求恰好一个活跃 Task")
        task = active[0]
        digest = sha256_bytes(repo.task_path(task["id"]).read_bytes())
        repo.write_handoff(repo.native_handoff(task, digest))
    return {"ok": True, "task_id": task["id"], "status": task["status"]}


def read_facts(repo: Repo, task_id: str) -> List[Dict[str, Any]]:
    path = repo.fact_path(task_id)
    if not path.exists():
        return []
    records, invalidated = [], set()
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if not line.strip():
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as exc:
            raise HarnessError(f"Fact ledger 第 {line_number} 行损坏：{path}") from exc
        if record.get("schema") != SCHEMA or record.get("kind") not in {"fact", "fact_invalidation"}:
            raise HarnessError(f"Fact ledger 第 {line_number} 行 schema/kind 无效：{path}")
        if record.get("kind") == "fact":
            clean_text(record.get("statement"), "Fact statement", MAX_TEXT, True)
            clean_text(record.get("source"), "Fact source", 1000, True)
            tags = record.get("tags", [])
            if not isinstance(tags, list) or len(tags) > 8:
                raise HarnessError(f"Fact tags 无效：{path}")
            for tag in tags:
                clean_text(tag, "Fact tag", 100, True)
        if record.get("kind") == "fact_invalidation":
            clean_text(record.get("invalidates"), "Fact invalidates", 200, True)
            clean_text(record.get("reason"), "Fact invalidation reason", 1000, True)
            invalidated.add(record.get("invalidates"))
        records.append(record)
    return [record for record in records if record.get("kind") == "fact" and record.get("id") not in invalidated]


def validate_decision(meta: Dict[str, Any], path: pathlib.Path) -> None:
    if meta.get("kind") != "decision":
        raise HarnessError(f"Decision kind 无效：{path}")
    for key, limit in (("title", 200), ("question", 1000), ("chosen", MAX_TEXT), ("source", 1000)):
        clean_text(meta.get(key), f"Decision {key}", limit, True)
    for key in ("rejected", "why_not"):
        values = meta.get(key, [])
        if not isinstance(values, list) or len(values) > 8:
            raise HarnessError(f"Decision {key} 无效：{path}")
        for value in values:
            clean_text(value, f"Decision {key}", 1000, True)


def read_decisions(repo: Repo, task_id: str) -> List[Dict[str, Any]]:
    decisions = []
    for path in repo.decisions.glob("dec_*.md"):
        meta = parse_markdown(path)[0]
        validate_decision(meta, path)
        if meta.get("task_id") == task_id:
            decisions.append(meta)
    return sorted(decisions, key=lambda item: (int(item.get("created_ns", 0)), item.get("created_at", ""), item.get("id", "")))


def native_projection_issues(repo: Repo, handoff: Dict[str, Any], task: Dict[str, Any]) -> List[str]:
    expected = repo.native_handoff(task, handoff.get("source_digest", ""))
    keys = ("mode", "engine", "task_id", "task_status", "source_path", "goal", "done", "next",
            "validation", "blockers", "must_read", "resume_command")
    return [key for key in keys if handoff.get(key) != expected.get(key)]


def verify_gsd_pointer(repo: Repo, handoff: Dict[str, Any]) -> None:
    source = repo.validate_reference(handoff.get("source_path") or ".planning/STATE.md",
                                     "GSD active_artifact", ".planning")
    digest = sha256_bytes((repo.root / source).read_bytes())
    if digest != handoff.get("source_digest"):
        raise HarnessError("GSD active_artifact 与 Handoff 摘要不一致；先刷新 GSD Handoff")


def limit_capsule(capsule: Dict[str, Any], decisions: List[Dict[str, Any]], facts: List[Dict[str, Any]]) -> Dict[str, Any]:
    selected_decisions = decisions[-3:]
    selected_facts = facts[-12:]
    omitted_decisions = max(0, len(decisions) - len(selected_decisions))
    omitted_facts = max(0, len(facts) - len(selected_facts))
    while True:
        capsule["decisions"] = selected_decisions
        capsule["facts"] = selected_facts
        capsule["omitted"] = {"decisions": omitted_decisions, "facts": omitted_facts}
        if len(json_dump(capsule).encode("utf-8")) <= CAPSULE_LIMIT:
            return capsule
        if selected_facts:
            selected_facts.pop(0)
            omitted_facts += 1
        elif selected_decisions:
            selected_decisions.pop(0)
            omitted_decisions += 1
        else:
            raise HarnessError(f"恢复胶囊基础状态超过 {CAPSULE_LIMIT} 字节")


def build_capsule(repo: Repo) -> Dict[str, Any]:
    handoff = repo.read_handoff()
    capsule = {key: handoff.get(key, [] if key in {"done", "next", "validation", "blockers", "must_read"} else "")
               for key in ("mode", "engine", "goal", "done", "next", "validation", "blockers", "must_read", "source_path", "resume_command")}
    capsule["schema"] = SCHEMA
    capsule["must_read"] = capsule["must_read"][:MAX_MUST_READ]
    if handoff.get("mode") == "native":
        task_id = handoff.get("task_id")
        raw = repo.task_path(task_id).read_bytes()
        if sha256_bytes(raw) != handoff.get("source_digest"):
            raise HarnessError("HANDOFF 与 Task 摘要不一致；运行 doctor，不要使用过期上下文继续")
        task = repo.read_task(task_id)
        projection_drift = native_projection_issues(repo, handoff, task)
        if projection_drift:
            raise HarnessError("HANDOFF 投影与 Task 不一致：" + ", ".join(projection_drift))
        capsule.update({
            "goal": task.get("goal", ""), "done": task.get("done", []), "next": task.get("next", []),
            "validation": task.get("validation", []), "blockers": task.get("blockers", []),
            "must_read": task.get("must_read", [])[:MAX_MUST_READ], "task_id": task_id,
            "status": task.get("status"), "profile": task.get("profile"), "risk": task.get("risk"),
            "revision": task.get("revision"), "acceptance": task.get("acceptance", []),
        })
        for reference in task.get("must_read", []):
            repo.validate_reference(reference, "must_read")
        return limit_capsule(capsule, read_decisions(repo, task_id), read_facts(repo, task_id))
    if handoff.get("mode") == "gsd":
        verify_gsd_pointer(repo, handoff)
    for reference in handoff.get("must_read", []):
        repo.validate_reference(reference, "must_read")
    if len(json_dump(capsule).encode("utf-8")) > CAPSULE_LIMIT:
        raise HarnessError(f"恢复胶囊超过 {CAPSULE_LIMIT} 字节")
    return capsule


def cmd_resume(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require_handoff()
    with repo.lock():
        return build_capsule(repo)


def cmd_status(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require_handoff()
    with repo.lock():
        handoff = repo.read_handoff()
        if handoff.get("mode") == "native":
            task, _ = active_task(repo)
            projection_drift = native_projection_issues(repo, handoff, task)
            if projection_drift:
                raise HarnessError("HANDOFF 投影与 Task 不一致：" + ", ".join(projection_drift))
            return {"ok": True, "root": str(repo.root), "mode": "native", "engine": "summer",
                    "task_id": task["id"], "task_status": task["status"], "goal": task["goal"],
                    "next": task.get("next", []), "updated_at": task.get("updated_at")}
        if handoff.get("mode") == "gsd":
            verify_gsd_pointer(repo, handoff)
        return {"ok": True, "root": str(repo.root), "mode": handoff.get("mode"), "engine": handoff.get("engine"),
                "task_id": handoff.get("task_id", ""), "task_status": handoff.get("task_status", ""),
                "goal": handoff.get("goal", ""), "next": handoff.get("next", []), "updated_at": handoff.get("updated_at")}


def cmd_doctor(args: argparse.Namespace, repo: Repo) -> Dict[str, Any]:
    repo.require_handoff()
    with repo.lock():
        issues, warnings = [], []
        if repo.handoff.stat().st_size > HANDOFF_LIMIT:
            issues.append("HANDOFF 超出大小限制")
        try:
            handoff = repo.read_handoff()
        except HarnessError as exc:
            return {"ok": False, "issues": [str(exc)], "warnings": warnings}
        if handoff.get("mode") not in {"idle", "direct", "native", "gsd"}:
            issues.append("HANDOFF mode 非法")
        active = []
        for path in repo.tasks.glob("task_*.md"):
            try:
                meta = parse_markdown(path)[0]
            except HarnessError as exc:
                issues.append(str(exc)); continue
            if meta.get("status") in ACTIVE_STATES:
                active.append(meta.get("id"))
        if handoff.get("mode") == "native":
            task_id = handoff.get("task_id")
            if active != [task_id]:
                issues.append("活跃 Task 与唯一 Handoff 不一致")
            if not task_id or not repo.task_path(task_id).exists():
                issues.append("HANDOFF 指向不存在的 Task")
            elif sha256_bytes(repo.task_path(task_id).read_bytes()) != handoff.get("source_digest"):
                issues.append("HANDOFF source_digest 与 Task 不一致")
            else:
                task = repo.read_task(task_id)
                drift = native_projection_issues(repo, handoff, task)
                if drift:
                    issues.append("HANDOFF 投影漂移：" + ", ".join(drift))
        elif active:
            issues.append("非 native Handoff 下存在活跃 Summer Task")
        if handoff.get("mode") == "gsd":
            try:
                verify_gsd_pointer(repo, handoff)
            except HarnessError as exc:
                issues.append(str(exc))
        for reference in handoff.get("must_read", []):
            try:
                repo.validate_reference(reference, "must_read")
            except HarnessError as exc:
                issues.append(str(exc))
        for path in repo.facts.glob("task_*.jsonl"):
            try:
                read_facts(repo, path.stem)
            except HarnessError as exc:
                issues.append(str(exc))
        for path in repo.decisions.glob("dec_*.md"):
            try:
                validate_decision(parse_markdown(path)[0], path)
            except HarnessError as exc:
                issues.append(str(exc))
        if (repo.root / "HANDOFF.md").exists():
            warnings.append("仓库根目录还存在另一个 HANDOFF.md；.agent/HANDOFF.md 才是权威入口")
        return {"ok": not issues, "issues": issues, "warnings": warnings, "active_tasks": active}


def add_many(parser: argparse.ArgumentParser, flag: str, help_text: str, required: bool = False) -> None:
    parser.add_argument(flag, action="append", default=[], required=required, help=help_text)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Summer Harness：显式启用的轻量文件型工作流")
    parser.add_argument("--repo", help="项目根目录；默认从当前目录向上查找")
    parser.add_argument("--json", action="store_true", help="以 JSON 输出")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("init", help="初始化 .agent/，不会创建任务")

    start = sub.add_parser("start", help="启动一个 Summer 原生任务")
    start.add_argument("--title", required=True); start.add_argument("--goal", required=True)
    add_many(start, "--acceptance", "可验证验收条件", True); add_many(start, "--next", "初始下一步")
    add_many(start, "--must-read", "恢复时必须读取的文件，最多五个")
    start.add_argument("--profile", choices=sorted(PROFILES), default="standard")
    start.add_argument("--risk", choices=sorted(RISKS), default="medium")

    checkpoint = sub.add_parser("checkpoint", help="更新进展并同步唯一 Handoff")
    add_many(checkpoint, "--done", "已完成工作"); add_many(checkpoint, "--next", "下一步")
    add_many(checkpoint, "--validation", "已执行的验证"); add_many(checkpoint, "--blocker", "当前阻塞")
    add_many(checkpoint, "--must-read", "恢复时必须读取的文件")
    checkpoint.add_argument("--clear-blockers", action="store_true")
    checkpoint.add_argument("--replace-done", action="store_true",
                            help="用本次 --done 有界摘要替换旧完成项；详细历史继续保留在 Fact/Decision/Git")

    fact = sub.add_parser("fact", help="追加可追溯 Fact，或使旧 Fact 失效")
    fact.add_argument("--statement"); fact.add_argument("--source")
    fact.add_argument("--confidence", choices=["low", "medium", "high"], default="high")
    fact.add_argument("--memory-class", choices=["session", "project", "durable"], default="project")
    add_many(fact, "--tag", "Fact 标签"); fact.add_argument("--invalidate"); fact.add_argument("--reason")

    decision = sub.add_parser("decision", help="记录具有长期影响的选择与拒绝方案")
    for flag in ("title", "question", "chosen", "source"):
        decision.add_argument(f"--{flag}", required=True)
    add_many(decision, "--rejected", "被拒绝方案"); add_many(decision, "--why-not", "拒绝原因")

    review = sub.add_parser("review", help="记录审查结论")
    review.add_argument("--summary", required=True); review.add_argument("--approved", action="store_true")
    review.add_argument("--reviewer", required=True, help="独立 reviewer 的 agent/session 标识")
    review.add_argument("--independent", action="store_true", help="声明 reviewer 未执行当前修订")
    add_many(review, "--finding", "未解决问题")

    complete = sub.add_parser("complete", help="通过风险门禁后完成任务")
    complete.add_argument("--summary", required=True); add_many(complete, "--validation", "收口验证")
    add_many(complete, "--residual-risk", "残余风险"); complete.add_argument("--ack-residual", action="store_true")

    handoff = sub.add_parser("handoff", help="为 Direct 或 GSD 写入唯一跨 session 交接")
    handoff.add_argument("--mode", choices=["direct", "gsd", "idle"], required=True); handoff.add_argument("--goal")
    add_many(handoff, "--done", "已完成工作"); add_many(handoff, "--next", "下一步")
    add_many(handoff, "--validation", "验证"); add_many(handoff, "--blocker", "阻塞")
    add_many(handoff, "--must-read", "必须读取"); handoff.add_argument("--active-artifact")
    handoff.add_argument("--resume-command")
    sub.add_parser("repair-handoff", help="从唯一活跃 Task 重建损坏或漂移的 native Handoff")
    sub.add_parser("resume", help="输出有界恢复胶囊"); sub.add_parser("status", help="输出当前模式和任务状态")
    sub.add_parser("doctor", help="检查账本、Handoff、指针和单写约束")
    return parser


COMMANDS = {"init": cmd_init, "start": cmd_start, "checkpoint": cmd_checkpoint, "fact": cmd_fact,
            "decision": cmd_decision, "review": cmd_review, "complete": cmd_complete,
            "handoff": cmd_handoff, "repair-handoff": cmd_repair_handoff,
            "resume": cmd_resume, "status": cmd_status, "doctor": cmd_doctor}


def human_output(command: str, result: Dict[str, Any]) -> str:
    if command in {"resume", "doctor"}:
        return json_dump(result)
    if command == "status":
        return f"mode={result['mode']} engine={result['engine']} status={result.get('task_status') or '-'} next={'；'.join(result.get('next', [])) or '无'}"
    return "ok " + " ".join(f"{key}={value}" for key, value in result.items() if key != "ok")


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser(); args = parser.parse_args(argv); repo = find_repo(args.repo)
    try:
        result = COMMANDS[args.command](args, repo)
        print(json_dump(result) if args.json else human_output(args.command, result))
        return 2 if args.command == "doctor" and not result.get("ok") else 0
    except (HarnessError, OSError) as exc:
        if args.json:
            print(json_dump({"ok": False, "error": str(exc)}))
        else:
            print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
