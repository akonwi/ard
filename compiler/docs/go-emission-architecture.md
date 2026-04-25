# Go Emission Architecture

This document describes the current architecture of Ard's Go backend.

The Go backend now uses a fully structured compilation pipeline for production transpilation. Generated Go code is no longer built through the legacy line-oriented source emitter, and production lowering no longer relies on captured Go source snippets that are parsed back into AST.

## Goals

- Separate semantic lowering from final source formatting.
- Keep the backend deterministic and easy to test.
- Use Go's structured syntax tooling (`go/ast`, `go/token`, `go/format`) instead of custom whole-file string emission.
- Preserve a clear pipeline shape with explicit lowering, optimization, and rendering stages.
- Leave room for future optimization and profiling work without reintroducing source-first emission.

## Current pipeline

```text
parse AST
→ checker checked tree
→ Go file IR
→ Go file IR optimization passes
→ go/ast
→ format.Node / gofmt-style output
→ generated .go files
```

For production Go-target transpilation, the main wrapper path is:

```text
CompileEntrypoint / compilePackageSource / compileModuleSource
→ lowerModuleFileIR / lowerAsyncModuleFileIR
→ optimizeGoFileIR
→ renderGoFile
→ formatGoFileAST
```

## Stage responsibilities

### 1. Checker checked tree

- The checker remains the semantic source of truth.
- Target-specific rules and extern resolution are completed before Go lowering begins.
- The Go backend consumes checked modules, checked statements, and checked expression/type metadata.

### 2. Go file IR

The current IR is intentionally lightweight and file-oriented:

- package name
- imports
- ordered declaration groups

This stage provides a stable place to:

- establish file/package structure
- collect and order imports
- preserve declaration ordering
- keep lowering and rendering separated by an explicit backend boundary

Today, most semantic lowering for declarations, statements, and expressions happens while constructing structured Go AST nodes that are then stored inside the file IR.

That means the current backend is best understood as:

- a lightweight file IR for module structure
- plus direct structured AST lowering for declaration and body contents

rather than a large standalone statement/expression IR.

### 3. Go file IR optimization passes

A lightweight optimization pass runs before rendering.

Current responsibilities include:

- import deduplication
- dropping empty declaration groups

This stage exists primarily as a pipeline slot for future cleanup and simplification work. It can grow to host additional optimizations without changing the overall backend shape.

### 4. `go/ast`

- Backend lowering produces structured Go syntax trees.
- Production transpilation lowers declarations and bodies directly to `ast.Decl`, `ast.Stmt`, and `ast.Expr` forms.
- `renderGoFile(...)` assembles those declarations into an `ast.File` with structured import specs.
- Go-specific surface syntax is handled through AST construction rather than reparsing generated source strings.

### 5. Formatting

- `formatGoFileAST(...)` uses `format.Node(...)` as the final rendering step.
- The formatted bytes are written as generated `.go` files.
- Final formatting is the only source-rendering step in production Go emission.

## Current state of the backend

The structured pipeline is now the production architecture, not an in-progress side path.

On this branch:

- every generated Go module lowers through `goFileIR`
- all production Go files render through `go/ast` + `go/format`
- user functions and methods lower directly to AST declarations
- package variables lower through the structured pipeline
- synthesized entrypoint `main` lowering is structured
- async support module generation lowers directly to AST
- the legacy builder/source emitter has been deleted
- production parse-back helpers for generated Go declarations/blocks have been removed
- function-body fallback emission has been removed
- package-var fallback emission has been removed
- synthesized top-level `main` fallback emission has been removed

In other words, production Go transpilation no longer depends on:

- `line(...)`
- builder-managed source indentation/output capture
- declaration-source capture followed by parse-back
- block-source capture followed by parse-back

Metadata strings such as package names, import paths, and generated symbol names still exist, but generated Go syntax is no longer emitted as raw source snippets and reparsed as part of production lowering.

## Architectural shape

The Go backend now matches the intended structured backend shape much more closely:

```text
checked Ard tree
→ Go file IR
→ optimized file IR
→ go/ast
→ formatted Go source
```

This gives the backend:

- a clear lowering boundary
- a distinct optimization stage
- structured rendering through Go-native tooling
- stage-level test and benchmark targets

## Future evolution

The current design still leaves room for further refinement.

Likely future directions include:

- expanding optimization passes beyond import/decl cleanup
- introducing richer backend-local IR where it proves useful for analysis or optimization
- improving compile-time characteristics of generated Go for large modules/programs
- adding more stage-specific profiling and benchmark coverage

If a richer IR is introduced later, it should extend the structured pipeline rather than reintroduce source-first emission.

## Success criteria

The backend should remain organized around this structured model:

```text
checked Ard tree
→ backend lowering
→ structured Go AST
→ formatted Go source
```

That architecture makes it easier to:

- profile transpilation stages
- optimize generated output
- reduce Go compile times
- test lowering independently from formatting
- evolve backend behavior without depending on string-based emit/parse cycles
