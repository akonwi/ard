# Ard Formatting Standard (Proposed)

This document proposes a staged formatter standard for Ard and introduces the first compiler command support.

## Goals

- Make Ard code style deterministic and machine-enforced.
- Reduce style-only diffs in reviews.
- Give editors and CI a single canonical formatter.

## Command

The compiler now includes:

- `ard format <file.ard>`: rewrites the file in place.
- `ard format --check <file.ard>`: exits non-zero when the file is not formatted.

## Phase 1 Rules (implemented)

Phase 1 is intentionally conservative so it is safe to adopt immediately:

1. Normalize line endings to LF (`\n`).
2. Remove trailing spaces and tabs from each line.
3. Ensure a trailing newline at end of non-empty files.

These rules are whitespace-only and do not change syntax structure.

## Phase 2 Candidate Rules (next)

After we have confidence in the command workflow, we can move to AST-aware formatting:

- Canonical indentation and brace placement.
- Spacing around operators, colons, commas, and arrows.
- Canonical multiline layout for structs, enums, function params, lists, maps, and match expressions.
- Import grouping and ordering.
- Comment anchoring and stable comment placement.

## Rollout Recommendation

1. Introduce `ard format --check` in CI on changed files.
2. Land a single formatting sweep commit to reduce merge churn.
3. Add editor integration once syntax-aware formatting lands.
