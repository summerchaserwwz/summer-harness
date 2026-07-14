import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import time
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
CLI = ROOT / "skills" / "summer-harness" / "scripts" / "harnessctl.py"


class HarnessTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.repo = pathlib.Path(self.temp.name)

    def tearDown(self):
        self.temp.cleanup()

    def invoke(self, *args, expected=0, env=None):
        command = [sys.executable, str(CLI), "--repo", str(self.repo), "--json", *args]
        result = subprocess.run(command, text=True, capture_output=True, env=env)
        self.assertEqual(result.returncode, expected, result.stderr + result.stdout)
        return json.loads(result.stdout) if result.stdout else {}

    def start(self, profile="standard"):
        self.invoke("init")
        return self.invoke("start", "--title", "实现功能", "--goal", "交付可验证结果",
                        "--acceptance", "测试通过", "--profile", profile)

    def rewrite_frontmatter(self, path, mutate):
        raw = path.read_text()
        match = re.match(r"\A---\s*\n(.*?)\n---\s*\n", raw, re.DOTALL)
        meta = json.loads(match.group(1))
        mutate(meta)
        path.write_text("---\n" + json.dumps(meta, ensure_ascii=False, indent=2, sort_keys=True) + "\n---\n" + raw[match.end():])

    def test_direct_handoff_is_resumable_without_task(self):
        (self.repo / "src").mkdir()
        (self.repo / "src" / "a.py").write_text("# context\n")
        self.invoke("handoff", "--mode", "direct", "--goal", "继续修复", "--done", "已定位原因",
                 "--next", "补测试", "--must-read", "src/a.py")
        capsule = self.invoke("resume")
        self.assertEqual(capsule["mode"], "direct")
        self.assertEqual(capsule["next"], ["补测试"])
        self.assertNotIn("task_id", capsule)
        files = sorted(str(path.relative_to(self.repo)) for path in (self.repo / ".agent").rglob("*") if path.is_file())
        self.assertEqual(files, [".agent/HANDOFF.md"])
        directories = sorted(str(path.relative_to(self.repo)) for path in (self.repo / ".agent").rglob("*") if path.is_dir())
        directories.insert(0, ".agent")
        self.assertEqual(directories, [".agent"])

    def test_native_task_checkpoint_and_complete(self):
        started = self.start()
        self.invoke("checkpoint", "--done", "实现完成", "--next", "运行测试", "--validation", "unit: pass")
        fact = self.invoke("fact", "--statement", "接口返回 200", "--source", "pytest::test_api")
        self.invoke("decision", "--title", "存储格式", "--question", "使用何种格式",
                 "--chosen", "Markdown frontmatter", "--source", "架构评审",
                 "--rejected", "SQLite", "--why-not", "默认过重")
        capsule = self.invoke("resume")
        self.assertEqual(capsule["task_id"], started["task_id"])
        self.assertEqual(capsule["facts"][0]["id"], fact["id"])
        self.assertEqual(len(capsule["decisions"]), 1)
        self.assertEqual(self.invoke("complete", "--summary", "交付完成")["status"], "done")
        self.assertEqual(self.invoke("status")["mode"], "idle")

    def test_completion_fails_without_validation(self):
        self.start()
        failed = self.invoke("complete", "--summary", "完成", expected=2)
        self.assertIn("缺少真实验证记录", failed["error"])

    def test_high_risk_requires_approved_review(self):
        self.start("high-risk")
        self.invoke("checkpoint", "--validation", "integration: pass")
        failed = self.invoke("complete", "--summary", "完成", expected=2)
        self.assertIn("缺少覆盖当前修订", failed["error"])
        self.invoke("review", "--summary", "独立审查通过", "--reviewer", "review-agent", "--approved", "--independent")
        self.invoke("complete", "--summary", "完成")

    def test_fact_invalidation_is_append_only(self):
        self.start()
        fact = self.invoke("fact", "--statement", "旧事实", "--source", "test")
        self.invoke("fact", "--invalidate", fact["id"], "--reason", "代码已变化")
        self.assertEqual(self.invoke("resume")["facts"], [])
        ledger = next((self.repo / ".agent" / "ledger" / "facts").glob("*.jsonl"))
        self.assertEqual(len(ledger.read_text().splitlines()), 2)

    def test_tampered_task_fails_closed(self):
        started = self.start()
        path = self.repo / ".agent" / "ledger" / "tasks" / f"{started['task_id']}.md"
        path.write_text(path.read_text() + "\nmanual edit\n")
        self.assertIn("摘要不一致", self.invoke("resume", expected=2)["error"])
        self.assertFalse(self.invoke("doctor", expected=2)["ok"])
        self.assertIn("摘要不一致", self.invoke("checkpoint", "--done", "不应覆盖", expected=2)["error"])

    def test_native_task_cannot_be_replaced_by_manual_handoff(self):
        self.start()
        failed = self.invoke("handoff", "--mode", "direct", "--goal", "覆盖", expected=2)
        self.assertIn("只能通过 checkpoint", failed["error"])

    def test_research_requires_sourced_fact(self):
        self.start("research")
        self.invoke("checkpoint", "--validation", "来源复核完成")
        failed = self.invoke("complete", "--summary", "研究完成", expected=2)
        self.assertIn("缺少带来源的有效 Fact", failed["error"])
        self.invoke("fact", "--statement", "结论", "--source", "https://example.test/source")
        self.invoke("complete", "--summary", "研究完成")

    def test_gsd_handoff_is_pointer_only(self):
        planning = self.repo / ".planning"
        planning.mkdir()
        (planning / "STATE.md").write_text("# state\n")
        self.invoke("handoff", "--mode", "gsd", "--goal", "完成多阶段项目", "--next", "恢复阶段")
        capsule = self.invoke("resume")
        self.assertEqual(capsule["engine"], "gsd")
        self.assertEqual(capsule["source_path"], ".planning/STATE.md")
        self.assertNotIn("facts", capsule)
        self.assertTrue(self.invoke("doctor")["ok"])

    def test_start_handoff_overflow_leaves_no_orphan(self):
        self.invoke("init")
        args = ["start", "--title", "大任务", "--goal", "G" * 900, "--acceptance", "通过"]
        for index in range(3):
            args.extend(["--next", str(index) + "N" * 999])
        failed = self.invoke(*args, expected=2)
        self.assertIn("HANDOFF 超过", failed["error"])
        self.assertEqual(list((self.repo / ".agent" / "ledger" / "tasks").glob("*.md")), [])
        self.assertEqual(self.invoke("status")["mode"], "idle")

    def test_checkpoint_overflow_rolls_back_both_files(self):
        started = self.start()
        task_path = self.repo / ".agent" / "ledger" / "tasks" / f"{started['task_id']}.md"
        handoff_path = self.repo / ".agent" / "HANDOFF.md"
        before = (task_path.read_bytes(), handoff_path.read_bytes())
        args = ["checkpoint"]
        for index in range(5):
            args.extend(["--done", str(index) + "D" * 999])
        self.assertIn("HANDOFF 超过", self.invoke(*args, expected=2)["error"])
        self.assertEqual((task_path.read_bytes(), handoff_path.read_bytes()), before)
        self.assertEqual(self.invoke("resume")["task_id"], started["task_id"])

    def test_checkpoint_can_replace_done_with_bounded_summary(self):
        self.start()
        self.invoke("checkpoint", "--done", "旧完成项一", "--done", "旧完成项二")
        self.invoke("checkpoint", "--replace-done", "--done", "M1-A 已完成并通过完整验证", "--next", "实现 Handoff projector")
        capsule = self.invoke("resume")
        self.assertEqual(capsule["done"], ["M1-A 已完成并通过完整验证"])
        self.assertEqual(capsule["next"], ["实现 Handoff projector"])
        self.assertLessEqual((self.repo / ".agent" / "HANDOFF.md").stat().st_size, 4096)

    def test_resume_rejects_handoff_over_4096_bytes(self):
        (self.repo / ".agent").mkdir()
        handoff = self.repo / ".agent" / "HANDOFF.md"
        handoff.write_bytes(b" " * 4097)
        failed = self.invoke("resume", expected=2)
        self.assertIn("HANDOFF 超过 4096 字节", failed["error"])

    def test_session_identity_is_hashed_before_persistence(self):
        env = os.environ.copy()
        env["CODEX_THREAD_ID"] = "019f-example-raw-session-id"
        self.invoke("init", env=env)
        started = self.invoke("start", "--title", "隐私", "--goal", "不公开原始 session id",
                              "--acceptance", "仅保存哈希别名", env=env)
        task_path = self.repo / ".agent" / "ledger" / "tasks" / f"{started['task_id']}.md"
        task = json.loads(re.match(r"\A---\s*\n(.*?)\n---\s*\n", task_path.read_text(), re.DOTALL).group(1))
        self.assertNotIn("019f-example-raw-session-id", task_path.read_text())
        self.assertRegex(task["created_by"], r"^session_[0-9a-f]{16}$")

    def test_interrupted_two_file_commit_rolls_forward(self):
        self.invoke("init")
        agent_dir = self.repo / ".agent"
        agent_dir.chmod(0o555)
        try:
            failed = self.invoke("start", "--title", "事务测试", "--goal", "验证恢复",
                                 "--acceptance", "可恢复", expected=2)
            self.assertIn("Permission denied", failed["error"])
        finally:
            agent_dir.chmod(0o755)
        status = self.invoke("status")
        self.assertEqual(status["mode"], "native")
        self.assertEqual(len(list((self.repo / ".agent" / "ledger" / "tasks").glob("*.md"))), 1)
        self.assertFalse((self.repo / ".agent" / "runtime" / "transaction.json").exists())

    def test_live_stale_looking_lock_cannot_be_stolen(self):
        holder = """
import importlib.util, pathlib, sys, time
spec = importlib.util.spec_from_file_location('summer_harness_cli', sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
repo = module.Repo(pathlib.Path(sys.argv[2]))
with repo.lock():
    print('locked', flush=True)
    time.sleep(30)
"""
        process = subprocess.Popen([sys.executable, "-c", holder, str(CLI), str(self.repo)],
                                   text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            self.assertEqual(process.stdout.readline().strip(), "locked")
            lock = self.repo / ".agent" / "runtime" / "write.lock"
            old = time.time() - 1000
            lock.touch()
            os.utime(lock, (old, old))
            failed = self.invoke("handoff", "--mode", "direct", "--goal", "不能抢锁", expected=2)
            self.assertIn("另一个 Harness 写入者", failed["error"])
        finally:
            process.terminate()
            process.communicate(timeout=5)
        self.invoke("handoff", "--mode", "direct", "--goal", "死亡锁可接管")

    def test_native_handoff_projection_tamper_fails_closed(self):
        self.start()
        handoff = self.repo / ".agent" / "HANDOFF.md"
        self.rewrite_frontmatter(handoff, lambda meta: meta.__setitem__("next", ["恶意下一步"]))
        self.assertIn("投影与 Task 不一致", self.invoke("resume", expected=2)["error"])
        self.assertFalse(self.invoke("doctor", expected=2)["ok"])
        repaired = self.invoke("repair-handoff")
        self.assertEqual(repaired["status"], "active")
        self.assertEqual(self.invoke("resume")["next"], [])

    def test_review_and_validation_are_bound_to_revision(self):
        self.start("high-risk")
        self.invoke("checkpoint", "--validation", "rev2 tests pass")
        self.invoke("review", "--summary", "rev2 approved", "--reviewer", "review-agent",
                    "--approved", "--independent")
        self.invoke("checkpoint", "--done", "审查后修改核心实现")
        failed = self.invoke("complete", "--summary", "不应完成", expected=2)
        self.assertIn("验证未覆盖当前任务修订", failed["error"])
        self.assertIn("缺少覆盖当前修订", failed["error"])

    def test_gsd_pointer_is_contained_and_digest_bound(self):
        missing = self.invoke("handoff", "--mode", "gsd", "--goal", "阶段项目", expected=2)
        self.assertIn("不存在", missing["error"])
        outside = self.invoke("handoff", "--mode", "gsd", "--goal", "阶段项目",
                              "--active-artifact", "/etc/passwd", expected=2)
        self.assertIn("仓库内相对路径", outside["error"])
        planning = self.repo / ".planning"
        planning.mkdir()
        state = planning / "STATE.md"
        state.write_text("v1\n")
        self.invoke("handoff", "--mode", "gsd", "--goal", "阶段项目")
        state.write_text("v2\n")
        self.assertIn("摘要不一致", self.invoke("resume", expected=2)["error"])
        self.assertFalse(self.invoke("doctor", expected=2)["ok"])

    def test_agent_symlink_is_rejected(self):
        with tempfile.TemporaryDirectory() as external:
            (self.repo / ".agent").symlink_to(external, target_is_directory=True)
            failed = self.invoke("handoff", "--mode", "direct", "--goal", "不能外写", expected=2)
            self.assertIn("不能是符号链接", failed["error"])
            self.assertFalse((pathlib.Path(external) / "HANDOFF.md").exists())

    def test_blank_fact_does_not_satisfy_research_gate(self):
        self.start("research")
        failed = self.invoke("fact", "--statement", "   ", "--source", "  ", expected=2)
        self.assertIn("不能为空", failed["error"])

    def test_capsule_has_strict_byte_limit(self):
        self.start()
        for index in range(12):
            self.invoke("fact", "--statement", f"{index}:" + "F" * 1900, "--source", f"source-{index}")
        for index in range(4):
            self.invoke("decision", "--title", f"D{index}", "--question", "Q" * 900,
                        "--chosen", "C" * 1800, "--source", "test")
        capsule = self.invoke("resume")
        self.assertLessEqual(len(json.dumps(capsule, ensure_ascii=False, indent=2, sort_keys=True).encode()), 32768)
        self.assertGreater(capsule["omitted"]["facts"] + capsule["omitted"]["decisions"], 0)

    def test_doctor_detects_torn_fact_and_decisions_are_recent(self):
        self.start()
        for index in range(6):
            self.invoke("decision", "--title", f"D{index}", "--question", "Q", "--chosen", "C", "--source", "test")
        self.assertEqual([item["title"] for item in self.invoke("resume")["decisions"]], ["D3", "D4", "D5"])
        self.invoke("fact", "--statement", "valid", "--source", "test")
        ledger = next((self.repo / ".agent" / "ledger" / "facts").glob("*.jsonl"))
        with ledger.open("a") as handle:
            handle.write('{"partial":')
        self.assertFalse(self.invoke("doctor", expected=2)["ok"])


if __name__ == "__main__":
    unittest.main()
