# Go Emission Architecture

This document defines the target architecture for Ard's Go backend.

The goal is to move the current transpiler from direct string emission toward a more formal compiler pipeline that is easier to optimize, profile, and evolve.

## Goals

- Separate semantic lowering from rendering.
- Make generated Go easier to optimize for compile time and output quality.
- Reuse Go's structured syntax tooling (`go/ast`, `go/token`, `go/format`) instead of relying on string concatenation as the final rendering model.
- Keep backend behavior deterministic and easy to snapshot-test.

## Pipeline

```text
parse AST
→ checker checked tree
→ Go IR
→ Go IR optimization passes
→ go/ast
→ format.Node / gofmt-style output
→ generated .go files
```

## Stage responsibilities

### 1. Checker checked tree

- The checker remains the semantic source of truth.
- Target-specific extern resolution and language rules are already handled before Go lowering begins.

### 2. Go IR

- Lower Ard semantics into a Go-oriented backend IR.
- This stage owns semantic decisions such as:
  - `try` lowering
  - `match` lowering
  - temp introduction and sequencing
  - pointer/value handling for mutable data
  - module import and helper usage decisions

### 3. Go IR optimization passes

- Run simple canonicalization and cleanup passes before rendering.
- Initial targets:
  - dead temp elimination
  - unreachable tail pruning
  - return simplification
  - import/helper pruning
  - control-flow normalization

### 4. `go/ast`

- Convert backend IR into structured Go syntax trees.
- This is the rendering boundary between backend semantics and surface syntax.
- Avoid mixing Ard-specific lowering logic into this stage.

### 5. Formatting

- Use `format.Node(...)` as the final source rendering step.
- Parsing or formatting errors should report the generated source to make backend debugging straightforward.

## Rollout plan

1. Split orchestration from emission logic.
2. Introduce a minimal Go IR for file/package/import structure.
3. Route final formatting through structured Go AST rendering.
4. Expand the IR to cover declarations, statements, and expressions.
5. Add optimization passes and stage-level benchmarks.

## Current incremental status

This repository is in the middle of that migration:

- frontend loading/build orchestration is separated from emission details
- module files now lower into a Go file IR that owns package/import/declaration ordering
- a Go IR optimization pass runs before rendering and is ready to host output simplification work
- rendered files flow through `go/ast` + `go/format`
- most expression and statement lowering inside declarations still uses the legacy direct emitter and will be migrated incrementally

## Success criteria

The Go backend should eventually have the same architectural shape as the JavaScript backend's intended pipeline, while using Go-native rendering tools:

```text
checked Ard tree
→ Go IR
→ go/ast
→ formatted Go source
```

That architecture should make it substantially easier to:

- profile transpilation stages
- optimize generated output
- reduce Go compile times
- test lowering independently from rendering
