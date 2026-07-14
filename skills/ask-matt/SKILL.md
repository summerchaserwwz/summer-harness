---
name: ask-matt
description: Manually choose among the small installed subset of Matt Pocock engineering skills. Use only when the user explicitly invokes ask-matt or asks which Matt skill fits. This local adapter never starts a workflow, creates state, or recommends uninstalled Matt orchestration skills.
---

# Ask Matt — Summer Subset

This is a local navigator for installed Matt capabilities, not the project lifecycle router.

Choose at most one primary skill:

- `diagnosing-bugs`: reproduce, minimise, hypothesise, instrument, fix, regression-test a hard bug or performance regression.
- `codebase-design`: design or improve module boundaries, interfaces, seams, and testability.
- `domain-modeling`: sharpen domain language, scenarios, `CONTEXT.md`, or architectural decisions.
- `tdd`: use a red-green-refactor loop when the user explicitly wants TDD or the task benefits from that feedback loop.

If none fit, say so and keep the task Direct. Do not invent or recommend `/implement`, `/wayfinder`, `/handoff`, `/to-spec`, `/to-tickets`, `/triage`, or other Matt skills that are not installed.

For lifecycle questions, defer without taking ownership:

- cross-session only: `$project-handoff`
- explicit persistent governance: `$summer-harness`
- explicit multi-phase fresh-context work: GSD
- route comparison: `$adaptive-harness-router`

Output only:

```text
Matt skill: <name | none>
Why: <one sentence>
Next: <the user's request restated with the selected skill>
```
