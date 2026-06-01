---
name: issue-tdd
description: Investigate and fix a GitHub issue using Ard's preferred TDD workflow. Use when asked to work on, investigate, fix, or create a branch/PR for a GitHub issue or bug report.
---

# Issue TDD Workflow

Use this workflow for Ard bug fixes and issue-driven implementation.

## 1. Read and summarize the issue

- Read the GitHub issue or linked local note completely.
- Identify:
  - expected behavior
  - observed behavior
  - minimal repro shape
  - affected layer: parser, checker, AIR, Go backend, JS backend, formatter, LSP, etc.
- Restate the suspected failure mode briefly before editing.

## 2. Triage by reproducing

Prefer a regression test over an ad-hoc sample.

- Add the smallest failing test in the most specific package:
  - parser syntax: `compiler/parse`
  - type checking / semantic diagnostics: `compiler/checker`
  - AIR shape: `compiler/air`
  - Go output/runtime behavior: `compiler/go`
  - JS output/runtime behavior: `compiler/javascript`
  - formatting: `compiler/formatter`
- Run only the new test first and confirm it fails for the expected reason.
- If a sample project is needed, create it in a temp dir and keep the command/output for the report.
- Do not proceed to implementation until the failure is reproduced.

## 3. Evaluate solutions

Before coding the fix, compare at least two plausible approaches when the solution is not obvious.

For each option, note:
- scope of change
- correctness/generalization
- regression risk
- compatibility with existing architecture

Prefer the smallest correct fix, but record larger architectural follow-ups as GitHub issues when appropriate.

## 4. Implement using TDD

- Make the failing test pass with a focused change.
- Avoid broad rewrites unless the issue requires one.
- Keep diagnostics explicit; do not silently drop errors.
- Preserve existing behavior unless the issue requires changing it.

## 5. Validate

Run targeted tests first, then broader tests as practical.

Typical commands from `compiler/`:

```sh
go test ./<package> -run <NewTestName> -count=1
go test ./parse ./checker ./air ./go -run '<relevant-pattern>' -count=1
go test ./... -count=1
```

If full test output exceeds the terminal timeout but package results are visible, report that accurately.

## 6. Report and handoff

Before committing, summarize:
- reproduction test added
- root cause
- chosen solution and rejected alternatives
- validation commands/results
- any untracked local repro notes not included

Only commit/push/create PR when explicitly asked. Use the `commit-format` skill before committing.
