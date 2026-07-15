#!/usr/bin/env python3
"""Mechanical checks for the Summer Harness v3 architecture contract."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


REQUIRED_FILES = [
    "docs/architecture-v3.md",
    "docs/product-spec-v3.md",
    "docs/data-model-v3.md",
    "docs/adr/0005-three-user-paths-and-authority-map.md",
    "docs/adr/0006-trust-journal-and-workref-bound-gates.md",
    "docs/adr/0007-capability-routing-and-single-coordinator.md",
    "docs/diagrams/summer-harness-v3.architecture.json",
    "docs/diagrams/summer-harness-v3.html",
    "config/AGENTS.md",
    "CONTEXT.md",
    "README.md",
    "skills/summer-harness/SKILL.md",
    "skills/summer-harness/references/contract.md",
    "skills/project-handoff/SKILL.md",
    "skills/adaptive-harness-router/SKILL.md",
]


REQUIRED_SNIPPETS = {
    "docs/architecture-v3.md": [
        "Direct",
        "Handoff Lite",
        "Governed GSD",
        "Authority Matrix",
        "Capability Router",
        "Coordinator lease",
        "Delivery Coverage Matrix",
        "Native v2 -> v3 Migration Contract",
        "### Legacy CLI 行为",
        "verified | limited | failed",
        "Git common-dir",
        "staged successor",
        "active migration fence",
        "target preimage",
        "Lite -> GSD Promotion Saga",
        "MigrationRollbackClosed",
        "Business Trust digest",
        "Contract Registry",
        "Promotion Control namespace",
        "USER_ACCEPTANCE_UNAVAILABLE",
    ],
    "docs/product-spec-v3.md": [
        "当前 v0.1 过渡表面",
        "目标命令在实现前必须标注“planned”",
        "Manual attestation 不能满足 machine-required Gate",
    ],
    "docs/data-model-v3.md": [
        "type WorkRef struct",
        "type WorkIdentityRef struct",
        "type SkillManifest struct",
        "type ContractRef struct",
        "type GateContract struct",
        "type ApprovedContractRecord struct",
        "type SkillPlanContract struct",
        "type SkillPlan struct",
        "type ActivitySkillPlan struct",
        "type ActorRef struct",
        "type SkillRef struct",
        "type GateSpec struct",
        "type UserInteractionProof struct",
        "type TrustedUserInteractionReceipt struct",
        "type HandoffSkillPlan struct",
        "type ObjectRef struct",
        "type ContentRef struct",
        "type ExactBackupRef struct",
        "type EnvironmentSummary struct",
        "type Finding struct",
        "type MigrationMapping struct",
        "type AssignmentCapsule struct",
        "type Proposal struct",
        "type ProposalReceipt struct",
        "type Evidence struct",
        "type Execution struct",
        "type Review struct",
        "type GateReceipt struct",
        "type UserAcceptanceReceipt struct",
        "type CompletionAuthorization struct",
        "type CancellationAuthorization struct",
        "type PromotionPlan struct",
        "type PromotionFence struct",
        "type PromotionPending struct",
        "type LitePromoted struct",
        "type PromotionRolledBack struct",
        "type MigrationRollbackClosed struct",
        "type CriterionCoverage struct",
        "type MigrationFence struct",
        "AuthorizedSuccessorDigest",
        "WorkingSetDigest",
        "RequiredGateSetDigest",
        "ContractApprovalDigest",
        "canonical_digest(GateSpecs)",
        "Result=failed",
        "SHA-256(schema + \"\\n\" + canonical_json)",
    ],
    "config/AGENTS.md": [
        "三路径路由",
        "多 Agent、多个活跃 Session、Phase/Wave/DAG 是 GSD 硬触发",
        "当前 v0.1 的 Native v2",
        "CompletionAuthorization",
    ],
    "docs/roadmap.md": [
        "## G0 — v3 Architecture Freeze",
        "## M2 — Trusted Delivery",
        "## M3 — Lifecycle/Capability Router and Coordinator",
        "## M4 — Governed GSD Adapter",
        "## M5 — On-demand GUI and Projections",
        "## M6 — Host Adapters and Controlled Evolution",
        "## M7 — Installation and Desktop",
        "## M8 — Open Source Release",
    ],
    "skills/summer-harness/SKILL.md": [
        "Do not start a new Native lifecycle",
        "exactly one primary and at most two supporting Skills",
        "one Coordinator lease under Git common-dir",
    ],
    "skills/adaptive-harness-router/SKILL.md": [
        "Legacy Native -> explicit migration",
        "never route new work into Native",
    ],
    "skills/project-handoff/SKILL.md": [
        "`direct` is a legacy read alias and the target writer emits `lite`",
    ],
    "docs/adr/0002-three-entry-deep-kernel.md": [
        "Engine 是深接口和 Trust/validation 边界，不是第二 Workflow owner",
    ],
    "docs/adr/0003-transaction-ledger-and-rebuildable-projections.md": [
        "status: superseded",
        "0005-three-user-paths-and-authority-map.md",
        "0006-trust-journal-and-workref-bound-gates.md",
    ],
    "docs/adr/0005-three-user-paths-and-authority-map.md": ["status: accepted"],
    "docs/adr/0006-trust-journal-and-workref-bound-gates.md": ["status: accepted"],
    "docs/adr/0007-capability-routing-and-single-coordinator.md": ["status: accepted"],
}


SUPERSEDED = {
    "docs/architecture-v2.md": "superseded_by: architecture-v3.md",
    "docs/product-spec-v2.md": "superseded_by: product-spec-v3.md",
    "docs/data-model-v2.md": "superseded_by: data-model-v3.md",
}


FORBIDDEN_OLD_ROUTING = {
    "config/AGENTS.md": ["`Summer native`：", "Summer native：`.agent/ledger/` 权威"],
    "README.md": ["显式要求 Summer：Native v2", "Native 使用 `.agent/ledger/`"],
    "skills/summer-harness/SKILL.md": ["Use `native` for a bounded Root Objective"],
    "skills/adaptive-harness-router/SKILL.md": ["Use `Summer native` only after explicit Harness authorization"],
    "skills/summer-harness/references/contract.md": [
        "| “使用 Summer Harness / 走 Harness” for bounded durable work | Summer native |"
    ],
}


EXPECTED_AUTHORITY_ROWS = {
    "代码、commit、tree": ("Git", "Worker"),
    "Lite 当前工作集": (".agent/HANDOFF.md", "Lite Writer"),
    "GSD Requirement/Phase/Plan/Wave/Task": (".planning/", "Coordinator"),
    "Evidence/Execution/Review/Gate/Completion/Cancellation Authorization": ("Summer Trust Journal", "Engine"),
    "Trusted user interaction / acceptance authorization": ("Summer Trust Journal", "Engine"),
    "SkillManifest/SkillPlan/Gate/Policy contract bytes": ("Contract Registry", "Engine"),
    "Promotion control records": ("Promotion Control namespace", "Engine"),
    "Migration control records": ("Migration Control namespace", "Engine"),
    "raw stdout/stderr/Artifact": ("private Evidence Store", "Evidence Module"),
    "Coordinator lease": ("Git common-dir runtime", "lease holder"),
    "Assignment/ActivitySkillPlan/Proposal/ingest receipt": ("Coordination namespace", "Engine"),
    "Proposal inbox envelope": ("Git common-dir runtime", "Worker"),
    "Handoff GSD pointer": (".agent/HANDOFF.md", "Coordinator"),
    "SQLite/FTS/Graph/GUI": ("SQLite/FTS/cache", "Projector"),
    "Skill route result": ("Handoff.current_skill_plan", "Engine"),
}


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def check_markdown_links(relative: str, text: str, errors: list[str]) -> None:
    base = (ROOT / relative).parent
    for target in re.findall(r"\[[^\]]+\]\(([^)]+)\)", text):
        if target.startswith(("http://", "https://", "#", "mailto:")):
            continue
        clean = target.split("#", 1)[0]
        if not clean:
            continue
        if not (base / clean).resolve().exists():
            errors.append(f"{relative}: broken local link {target}")


def check_authority_matrix(text: str, errors: list[str]) -> None:
    match = re.search(r"^## Authority Matrix\s*$\n(.*?)(?=^## )", text, re.MULTILINE | re.DOTALL)
    if not match:
        errors.append("architecture: Authority Matrix section is missing")
        return

    rows: dict[str, tuple[str, str]] = {}
    for line in match.group(1).splitlines():
        if not line.startswith("|") or line.startswith("|---") or "| 事实 |" in line:
            continue
        cells = [cell.strip() for cell in line.strip().strip("|").split("|")]
        if len(cells) != 6:
            errors.append(f"architecture: malformed Authority Matrix row: {line}")
            continue
        fact, authority, writer = cells[0], cells[1], cells[2]
        if fact in rows:
            errors.append(f"architecture: duplicate fact row {fact!r}")
            continue
        if not authority or not writer:
            errors.append(f"architecture: fact {fact!r} lacks authority/writer")
        rows[fact] = (authority, writer)

    for fact, (required_authority, required_writer) in EXPECTED_AUTHORITY_ROWS.items():
        if fact not in rows:
            errors.append(f"architecture: missing Authority Matrix fact {fact!r}")
            continue
        authority, writer = rows[fact]
        if required_authority not in authority:
            errors.append(
                f"architecture: fact {fact!r} authority {authority!r} does not contain {required_authority!r}"
            )
        if required_writer not in writer:
            errors.append(
                f"architecture: fact {fact!r} writer {writer!r} does not contain {required_writer!r}"
            )

    skill_authority = rows.get("Skill route result", ("", ""))[0]
    if "GSD Coordination `ActivitySkillPlan`" not in skill_authority:
        errors.append("architecture: Skill route authority does not include GSD ActivitySkillPlan")

    planning_authorities = [fact for fact, (authority, _) in rows.items() if authority == "`.planning/`"]
    if planning_authorities != ["GSD Requirement/Phase/Plan/Wave/Task"]:
        errors.append(f"architecture: .planning authority is not unique: {planning_authorities}")


def check_no_workflow_mirroring(data_model: str, errors: list[str]) -> None:
    current = data_model.split("## Legacy v2 Mapping", 1)[0]
    for pattern in [r"type\s+RootObjective\b", r"type\s+WorkItem\b", r"\bTaskStatus\b", r"\bPhaseStatus\b", r"\bPlanStatus\b", r"\bWaveStatus\b"]:
        if re.search(pattern, current):
            errors.append(f"data model mirrors Workflow state outside legacy mapping: {pattern}")

    handoff_match = re.search(r"type Handoff struct \{(.*?)\n\}", current, re.DOTALL)
    if not handoff_match:
        errors.append("data model: Handoff struct is missing")
    else:
        if "// direct|" in handoff_match.group(1):
            errors.append("data model: direct must not be a v3 persistent Handoff mode")
        for field in ["TaskStatus", "PhaseStatus", "PlanStatus", "WaveStatus"]:
            if field in handoff_match.group(1):
                errors.append(f"data model: Handoff illegally mirrors {field}")

    for type_name in ["Handoff", "Evidence", "Execution", "Review", "GateReceipt"]:
        match = re.search(rf"type {type_name} struct \{{(.*?)\n\}}", current, re.DOTALL)
        if not match:
            errors.append(f"data model: {type_name} struct is missing")
            continue
        mirrored = re.findall(r"\b(?:Task|Phase|Plan|Wave)(?:ID|Status|State|Progress)\b", match.group(1))
        if mirrored:
            errors.append(f"data model: {type_name} mirrors GSD workflow fields: {sorted(set(mirrored))}")


def main() -> int:
    errors: list[str] = []

    for relative in REQUIRED_FILES:
        if not (ROOT / relative).is_file():
            errors.append(f"missing required file: {relative}")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    for relative, snippets in REQUIRED_SNIPPETS.items():
        text = read(relative)
        for snippet in snippets:
            if snippet not in text:
                errors.append(f"{relative}: missing contract snippet {snippet!r}")

    for relative, marker in SUPERSEDED.items():
        text = read(relative)
        if "status: superseded" not in text or marker not in text:
            errors.append(f"{relative}: missing superseded marker/pointer")

    for relative, patterns in FORBIDDEN_OLD_ROUTING.items():
        text = read(relative)
        for pattern in patterns:
            if pattern in text:
                errors.append(f"{relative}: old active routing remains: {pattern!r}")

    architecture = read("docs/architecture-v3.md")
    check_authority_matrix(architecture, errors)
    check_no_workflow_mirroring(read("docs/data-model-v3.md"), errors)

    if "Handoff 不超过 4 KiB" not in architecture and "文件不超过 4 KiB" not in architecture:
        errors.append("architecture does not declare the 4 KiB Handoff limit")
    if "must_read" not in architecture or "32 KiB" not in architecture:
        errors.append("architecture does not declare must_read/Capsule limits")
    if "一个 primary" not in architecture or "两个 supporting" not in architecture:
        errors.append("architecture does not declare the 1+2 SkillPlan limit")

    for relative in [
        "README.md",
        "docs/architecture.md",
        "docs/architecture-v3.md",
        "docs/product-spec-v3.md",
        "docs/data-model-v3.md",
        "docs/roadmap.md",
        "skills/summer-harness/SKILL.md",
        "skills/project-handoff/SKILL.md",
        "skills/adaptive-harness-router/SKILL.md",
    ]:
        check_markdown_links(relative, read(relative), errors)

    scoped_text = "\n".join(read(path) for path in REQUIRED_FILES if path.endswith((".md", ".json", ".html")))
    for forbidden in ["/Users/summer/", "sk-proj-", "github_pat_", "BEGIN PRIVATE KEY"]:
        if forbidden in scoped_text:
            errors.append(f"architecture artifacts contain forbidden private content: {forbidden}")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(
        "architecture contract: OK "
        f"({len(REQUIRED_FILES)} files, {sum(len(v) for v in REQUIRED_SNIPPETS.values())} required snippets)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
