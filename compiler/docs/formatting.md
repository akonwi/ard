# Ard Formatting Standard

This document defines the canonical formatting standard enforced by `ard format`.

## Goals

- Make Ard code style deterministic and machine-enforced.
- Reduce style-only diffs in reviews.
- Give editors and CI a single canonical formatter.

## Command

The compiler includes:

- `ard format <path>`: formats a single `.ard` file or recursively formats all `.ard` files under a directory.
- `ard format --check <path>`: exits non-zero when formatting changes are needed.

## Core Rules

The formatter is AST-aware and enforces these rules:

1. Line width target: 100 columns.
2. Indentation: 2 spaces.
3. Brace style: K&R (opening brace on the same line).
4. Operator spacing: spaces around binary operators.
5. Range spacing: no spaces around `..`.
6. Colon spacing: no space before, one space after (`name: Str`, `arg: 1`).
7. Trailing commas: required for multiline lists, maps, params, args, and enum variants.
8. Function parameter wrapping: one parameter per line when wrapped.
9. Match case layout: inline only when case body is a single expression that fits line width.
10. Import grouping and ordering:
   - `ard/*`
   - absolute package paths
   - relative imports (`./`, `../`)
   Each group is sorted alphabetically and separated by one blank line.
11. Whitespace normalization:
   - line endings normalized to LF (`\n`)
   - trailing spaces and tabs removed
   - trailing newline added for non-empty files
12. Comments: conservatively preserved (re-indented as needed).

## Rollout Recommendation

1. Run `ard format` once across the repository and commit the result in a dedicated formatting commit.
2. Enforce `ard format --check` in CI.
3. Configure editor integration to run formatter on save.
