# 0004: Use Canonical Ard Formatting

## Status

Accepted

## Context

Ard source should be easy to read, deterministic to review, and consistent across the monorepo. Without a canonical formatter and formatting standard, contributors and editor integrations can produce style-only diffs that obscure semantic changes.

The compiler includes `ard format` for formatting Ard source and `ard format --check` for detecting formatting drift without modifying files.

## Decision

Use `ard format` as the canonical formatter for Ard source files.

The formatter should be AST-aware and enforce one project-wide style.

Formatting should use a document-based Wadler/Prettier-style architecture:

1. parse source to AST
2. lower AST to a formatting document IR
3. print the document with a line-fitting algorithm

The document IR should describe layout choices rather than directly writing text. Core document operations include text, concatenation, groups, soft/hard lines, indentation, and conditional break output. Groups are printed flat when they fit within the configured width and broken across lines otherwise. This keeps wrapping decisions composable and avoids ad hoc per-node width checks.

The durable formatting choices are:

- 100-column target line width
- 2-space indentation
- K&R brace style, with opening braces on the same line
- spaces around binary operators
- no spaces around range operators (`..`)
- colon spacing with no space before and one space after (`name: Str`, `arg: 1`)
- required trailing commas for multiline lists, maps, params, args, and enum variants
- one function parameter per line when parameters wrap
- inline match cases only when the case body is a single expression that fits the line width
- grouped and sorted imports in this order:
  1. `ard/*`
  2. absolute package paths
  3. relative imports (`./`, `../`)
- whitespace normalization to LF line endings, no trailing spaces/tabs, and trailing newline for non-empty files
- conservative comment preservation

Use `ard format <path>` for one-off formatting. Use `ard format --check <path>` in CI or pre-submit checks when formatting drift should fail the job.

## Consequences

- Ard code style is deterministic and machine-enforced.
- Reviews should contain fewer style-only diffs.
- Editors and CI have a single formatting authority.
- Formatter changes can create broad diffs and should be reviewed as formatting changes, preferably separate from semantic changes.
- The formatter implementation must keep wrapping decisions composable through document groups and fitting checks.
- The formatter implementation must preserve comments conservatively and keep formatting idempotent.
- Perfect comment placement preservation and lossless source reconstruction are not required by the current formatter contract.

## Related

- `docs/adrs/0001-record-architecture-decisions.md`
