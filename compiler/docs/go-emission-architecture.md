# Go Emission Architecture

This document describes Ard's current Go backend architecture.

The Go backend is now IR-first end to end for production transpilation. The old checker-backed Go AST emitter has been removed. Production code generation lowers checked Ard modules into backend IR, renders that IR into Go AST, and formats the result with Go's standard tooling.

## Pipeline

```text
parse AST
→ checker checked tree
→ backend IR
→ Go file IR
→ go/ast
→ go/format
→ generated .go files
```

For production Go-target transpilation, the main path is:

```text
CompileEntrypoint / compilePackageSource / compileModuleSource
→ lowering.LowerModuleToBackendIR(...)
→ emitGoFileFromBackendIR(...)
→ optimizeGoFileIR(...)
→ renderGoFile(...)
```

## Stage responsibilities

### 1. Checker checked tree

The checker remains the semantic source of truth.

It is responsible for:
- name resolution
- type checking
- target-aware semantic shaping before backend lowering

### 2. Backend IR

`compiler/go_backend/ir` is the backend-owned semantic boundary.

It models:
- module/package identity
- Go import metadata needed by emission
- required imported Ard module paths for generated-project writing
- declarations: structs, enums, unions, extern types, functions, vars
- statement/expression structure for function bodies and entrypoint execution
- backend-specific metadata such as trait types/coercions, method ownership, and by-ref mutable params

`compiler/go_backend/lowering` is responsible for lowering checked modules into this IR.

### 3. Go file assembly

`goFileIR` is a lightweight file-assembly stage that organizes lowered backend IR output into target Go files.

It is responsible for:
- package clause
- import block assembly
- ordered top-level declaration grouping

It is not a second semantic IR. Semantic decisions should already be made by backend IR lowering and backend IR emission; this stage only organizes emitted Go declarations into final file structure.

### 4. Go AST emission

`compiler/go_backend/backend_ir_emit.go` renders backend IR directly into `go/ast` nodes.

This stage is responsible for:
- type declaration emission
- function/method emission
- entrypoint `main` synthesis
- native lowering of backend IR statements/expressions into Go syntax

There is no production fallback to a separate checker-backed emitter.

### 5. Formatting

`renderGoFile(...)` and `format.Node(...)` perform final Go source rendering.

Formatting is the only source-text emission step.

## Current state

The backend now has these properties:

- production transpilation is backend-IR-first
- the old checker→AST emitter path has been deleted
- imports and generated-project dependency discovery are produced during lowering and carried on backend IR module metadata
- declaration emission, body emission, and synthesized entrypoint emission are all native backend IR render paths

## Generated project writing

When the Go backend writes a generated Go project, it:

1. lowers the checked Ard module to backend IR
2. compiles the entrypoint module from backend IR
3. uses backend IR module metadata to determine which imported Ard modules must also be written
4. recursively compiles those imported modules through the same IR-first path

This keeps project emission aligned with the same backend boundary used by direct module compilation.

## Design goals

This architecture is intended to keep the Go backend:
- deterministic
- structurally testable
- easy to profile by stage
- explicit about unsupported cases
- free of source-snippet parse-back or hidden legacy fallback behavior

## Success criteria

The intended long-term shape is:

```text
checked Ard tree
→ backend IR lowering
→ native backend IR emission
→ go/ast
→ formatted Go source
```

