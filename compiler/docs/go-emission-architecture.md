# Go Emission Architecture

This document describes Ard's current Go backend architecture.

The current Go target lives under `compiler/go` and is built directly on AIR.
The previous Go backend implementation and its backend-specific IR have been
removed.

The Go target is intended to be a real Ard backend, not a partial transpiler.
It supports `ard run --target go` and `ard build --target go` by lowering AIR
into Go AST, formatting the generated source with Go's standard tooling, and
then invoking the Go toolchain.

## Pipeline

```text
Ard source
→ parse
→ checker
→ AIR
→ compiler/go lowering
→ go/ast
→ go/format
→ generated Go source
→ go build / go run-equivalent binary execution
```

At the public API level, the backend surface is:

- `GenerateSources(program, options)`
- `RunProgram(program, args)`
- `BuildProgram(program, outputPath)`

## Stage responsibilities

### 1. Checker and AIR

The checker remains the semantic source of truth for Ard programs.

It is responsible for:
- parsing/import resolution inputs needed for semantic analysis
- name resolution
- type checking
- shaping the checked program for AIR lowering

AIR is the backend input boundary.

The Go backend does not lower from parse ASTs, checker nodes, legacy bytecode
structures, or the deleted `compiler/go_backend` implementation.

### 2. AIR to Go lowering

`compiler/go/lower.go` lowers `air.Program` directly into Go AST fragments plus
backend-owned helper state.

This lowering stage is responsible for:
- deterministic naming of generated symbols
- per-module file assembly
- import planning based on actual generated usage
- Ard-to-Go type mapping
- statement-oriented lowering for expression-oriented Ard constructs
- extern call lowering
- generation of small backend-owned helper functions where needed

The backend does not introduce a separate Go-specific semantic IR today. The
main working model is direct AIR-to-Go lowering with temporary backend state.

### 3. Go AST rendering and formatting

`compiler/go/render.go` renders the generated Go AST through `go/format`.

Formatting is the final source emission step. The backend does not maintain a
string-template emitter for production codegen.

### 4. Toolchain execution

`RunProgram` and `BuildProgram` materialize a generated Go workspace, write the
formatted source files, synthesize a `go.mod`, and invoke `go build`.

- `RunProgram` builds a temporary executable inside the generated workspace and
  runs it with the forwarded Ard CLI program arguments.
- `BuildProgram` builds the final binary at the requested output path.

## Output structure

The backend emits:

- one generated Go package per Ard project
- one generated Go file per Ard module

This keeps Ard module boundaries visible in generated code while avoiding a
multi-package emission strategy.

## Artifact workspace layout

Generated workspaces are preserved under a project-local output directory:

- `ard-out/go/run`
- `ard-out/go/build`

Each new run or build clears and rewrites the corresponding directory.

This layout is intended for inspection and debugging, not for stable checked-in
artifacts.

## Lowering approach

The Go backend lowers Ard semantics into explicit Go semantics. It does not try
to preserve Ard's surface syntax mechanically when a statement-oriented Go shape
is clearer or more correct.

In practice, complex AIR expressions often lower into:

- setup statements
- temporaries
- a final Go expression or assigned result
- explicit control flow and early returns

This is especially important for:

- block expressions
- `if` expressions
- `match`
- `try`
- short-circuiting flows
- async/fiber coordination

## Type mapping

The backend prefers plain Go representations where that keeps the generated code
clear, but uses a few shared runtime types where they encode Ard semantics more
cleanly.

Current default mapping:

```text
Int          -> int
Float        -> float64
Bool         -> bool
Str          -> string
[T]          -> []T
[K:V]        -> map[K]V where K is representable as a Go map key
struct       -> generated Go struct
enum         -> generated named integer type
union        -> generated tagged Go struct
T?           -> runtime.Maybe[T]
T!E          -> runtime.Result[T, E]
Dynamic      -> any
extern type  -> any
trait object -> any where required by the current lowering surface
fn(...) ...  -> native Go function
Fiber[T]     -> generated/runtime-backed typed fiber handle
```

Small generated helpers are still used where needed, but the default rule is to
lower to native Go values first and only introduce helpers when Ard semantics or
host interop make that worthwhile.

## Runtime helper policy

The backend intentionally keeps its helper surface small.

Shared runtime usage is centered on:

- `github.com/akonwi/ard/runtime.Maybe[T]`
- `github.com/akonwi/ard/runtime.Result[T, E]`

Generated helper functions may also be emitted for specific backend needs such
as:

- fiber coordination
- deterministic key ordering helpers
- narrow dynamic conversion helpers used by host/decode paths
- stdin parsing or similar execution support

The backend should not regress toward a universal object runtime or VM-shaped
value layer.

## Stdlib and FFI model

Pure Ard stdlib modules compile through the normal AIR pipeline into generated
Go.

Extern-backed stdlib and host capability modules are lowered as direct static Go
calls wherever possible. The preferred model is:

- direct calls into the Ard Go FFI surface
- generated wrappers only when required for representation adaptation
- native Go closures for callback-shaped externs
- opaque host values carried as `any`

The Go target intentionally avoids VM-style concepts such as:

- extern registration maps
- dynamic binding registries
- callback handle tables
- general runtime object conversion layers

If a host boundary is awkward, the preferred fix is usually to normalize the Go
FFI surface rather than add more backend-specific adapter machinery.

## Current architectural decisions

The following decisions are intentional and should be treated as current design
constraints unless replaced explicitly:

- The backend input boundary is `air.Program`.
- The backend lowers directly from AIR to Go AST.
- Generated Go is rendered through `go/format`.
- The backend emits one package per Ard project and one file per Ard module.
- `Maybe` lowers as `runtime.Maybe[T]`.
- `Result` lowers as `runtime.Result[T, E]`.
- `Dynamic` lowers as plain Go `any`.
- Opaque extern values lower as `any`.
- Extern calls should be direct static Go calls by default.
- Generated artifacts live under `ard-out/go/{run,build}` and are overwritten
  on each new run/build.

## Non-goals

The current Go backend should not:

- reintroduce the deleted `compiler/go_backend` architecture
- introduce a universal `runtime.Object` representation for generated Go
- model all Ard values as `any`
- copy `vm_next`'s registry-driven host adapter model into generated Go
- preserve Ard surface syntax at the expense of correct and maintainable Go
  lowering

## Related work

Open rollout work for the Go target is tracked in `TODO.md`.
