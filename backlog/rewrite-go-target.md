# Rewrite the Go Target

This backlog tracks a clean replacement for Ard's Go target after AIR and
`vm_next` parity. The intent is to start over from AIR, not to refactor the
current Go emitter into shape.

## Ultimate goal

The end state is that `ard run --target go` and `ard build --target go` can run
existing Ard programs with the same broad coverage users expect from the primary
runtime targets.

That means the rewritten Go target should eventually be able to execute:

- normal Ard applications
- sample programs
- test programs
- stdlib-driven programs
- real projects that currently run on the VM targets

In short: the Go target should become a real backend for Ard, not a partial or
experimental transpiler.

## Status

In progress

## Context

AIR now gives Ard a typed, target-neutral representation that is independent of
the bytecode VM and independent of `runtime.Object`. `vm_next` proves that the
language can execute against AIR with signature-driven FFI and target-shaped
values.

The rewritten Go target should use that same AIR input so Ard is not merely
syntactic sugar over Go, while still generating idiomatic Go where the target
supports it.

The current Go target should be treated as disposable. It can be used as a source
of examples or test cases, but the new implementation should not inherit its
architecture, runtime assumptions, or lowering strategy by default.

## Goals

- Build the Go target from scratch against AIR.
- Use a real compiler pipeline: `AIR -> Go AST -> go/format`.
- Emit idiomatic Go where the target supports a direct mapping.
- Generate one Go file per Ard module.
- Generate one Go package for the whole Ard project.
- Generate native Go values for Ard scalars and containers.
- Generate Go structs for Ard structs crossing backend and FFI boundaries.
- Generate tagged representations for Ard unions.
- Generate native Go closures for ordinary Ard function values.
- Lower Ard fibers to goroutines plus typed fiber/result handles.
- Coalesce self-hosted Ard stdlib modules into generated Go.
- Emit direct Go calls or thin generated wrappers for low-level externs.
- Preserve Ard's testing model and single-file executable workflow where
  possible.
- Reach the point where `--target go` can run all existing Ard programs.

## Non-goals

- Do not preserve the current Go target's implementation structure.
- Do not incrementally patch the current Go target into the AIR model.
- Do not introduce a dedicated Go-specific IR unless direct AIR-to-Go-AST
  lowering proves insufficient later.
- Do not make `runtime.Object` part of generated Go.
- Do not model every Ard value as `any`.
- Do not copy VM-oriented extern registries, callback handle tables, or
  adapter registration flows into the Go backend when direct Go calls are
  available.
- Do not force `vm_next` callback handles into generated Go when native Go
  closures are available.

## Pipeline

The new backend should be a proper compiler pipeline, parallel to `vm_next` and
`compiler/javascript`:

```text
Ard source
  -> parse
  -> checker
  -> AIR
  -> Go backend lowering
  -> Go AST
  -> go/format
  -> generated Go source / built binary
```

The Go backend should take `air.Program` as its semantic input. It should not
rely on checker nodes, parse AST, old bytecode structures, or the deleted Go
backend architecture.

A separate Go-specific IR is not planned as part of the rewrite. The initial
implementation should lower AIR directly into Go AST, with backend-owned helper
state for imports, symbols, files, and expression lowering.

## Output structure

The default output structure should be:

- one generated Go package per Ard project
- one generated Go file per Ard module

This keeps module boundaries visible in generated code while avoiding the
complexity of a multi-package Go emission strategy during the rewrite.

## Lowering approach

The backend should lower Ard semantics into Go semantics explicitly.

It should not try to preserve expression-oriented Ard syntax when Go's
statement-oriented structure is a better fit. When needed, AIR expressions
should lower into:

- setup statements
- intermediate temporaries
- a final Go expression or value
- explicit early returns

That approach is especially important for:

- block expressions
- `if` expressions
- `try`
- match lowering
- short-circuiting flows

The working mental model is:

```go
type loweredExpr struct {
    Stmts []ast.Stmt
    Expr  ast.Expr
}
```

The generated Go should be pure and idiomatic where possible. Ard semantics such
as `try` should become explicit Go control flow, temporary values, and early
returns rather than being forced through runtime abstractions.

## Type Mapping

Initial Go mapping should follow the AIR design, but the backend should prefer
plain Go representations where they make the generated code clearer and more
idiomatic:

```text
Int          -> int
Float        -> float64
Bool         -> bool
Str          -> string
[T]          -> []T
[K:V]        -> map[K]V for Ard-keyable K
struct       -> generated Go struct
enum         -> generated named integer type
T?           -> plain Go shape where practical, likely `(T, bool)`
T!E          -> plain Go shape where practical, often `(T, error)`
union        -> generated tagged representation
Dynamic      -> opaque dynamic representation behind decode APIs
extern type  -> host resource handle
fn(...) ...  -> native Go function
Fiber[T]     -> generated/runtime fiber handle
```

Small runtime helper types may still be useful in some cases, but the default
rule should be:

> lower to plain Go first; introduce runtime helpers only when they materially
> improve the generated code or are required to preserve Ard semantics cleanly.

## Stdlib Model

Pure Ard stdlib modules should compile from AIR into Go. Host capability modules
should become direct Go calls or generated adapters.

Examples:

- `ard/list`, `ard/map`, `ard/result`, `ard/maybe`, decode helpers, routing, and
  pure protocol logic should compile from Ard where possible.
- filesystem, environment, process, clock, crypto, sockets, JSON parse/stringify,
  and low-level HTTP hooks remain target externs.

## FFI Model

The Go target should take advantage of the fact that the output is already Go.
Unlike `vm_next`, it should not need a registry-based or VM-style adapter layer
for normal extern calls.

The preferred model is:

- direct Go calls for externs by default
- generated structs as the default boundary type
- opaque externs for host resources that do not fit Ard's type system
- native Go closures for callback boundaries
- thin generated wrappers only when needed to reconcile Ard semantics with the
  chosen Go representation

That means the Go backend should avoid rebuilding VM-oriented concepts such as:

- extern registration maps
- dynamic binding registries
- callback handle tables
- generalized runtime conversion layers

Where an extern already matches the generated Go surface, the backend should
emit a direct static Go call.

## Milestones

### Milestone 1: backend skeleton

Status: In progress

- [x] Recreate the Go target implementation under `compiler/go/` with a clean
      package layout.
- [x] Add the public backend entrypoints for generate/run/build.
- [x] Lower AIR directly into Go AST.
- [x] Render generated Go through `go/format`.
- [x] Generate one Go file per Ard module.
- [x] Generate one Go package per Ard project.
- [x] Establish deterministic symbol naming.
- [x] Support a tiny end-to-end subset: constants, locals, arithmetic, direct
      function calls, and `main`.
- [ ] Expand import planning beyond the current minimal subset.
- [ ] Define the extern lowering model in code, even if initial support is
      stubbed.

### Milestone 2: structured control flow

Status: In progress

- [x] Lower block expressions through temporaries and statement setup for the
      current scalar subset.
- [x] Lower `if` expressions and `if` statements for the current scalar subset.
- [x] Lower assignments and mutable locals for the current scalar subset.
- [x] Lower loops and control-flow-heavy statement bodies for the current
      scalar subset.
- [ ] Expand control-flow lowering coverage beyond the current scalar subset.
- [ ] Lower `try` into explicit intermediate values and early returns.

### Milestone 3: core data model

Status: In progress

- [x] Generate Ard structs as Go structs for the current subset.
- [x] Generate enums for the current subset.
- [ ] Lower lists and maps to native Go containers where representable.
- [ ] Settle the default Go lowering for `Maybe`.
- [ ] Settle the default Go lowering for `Result`.
- [ ] Keep runtime helpers minimal and justify each one.

### Milestone 4: advanced language features

- [ ] Generate tagged unions.
- [ ] Generate native Go closures and capture handling.
- [ ] Lower trait dispatch where dynamic trait objects are required.
- [ ] Lower fibers to goroutines and typed handles.

### Milestone 5: FFI and stdlib integration

- [ ] Lower extern calls to direct Go calls by default.
- [ ] Generate thin wrappers only where direct calls are insufficient.
- [ ] Support generated structs across the Go extern boundary.
- [ ] Support opaque extern types.
- [ ] Support callback externs as native Go closures.
- [ ] Compile self-hosted stdlib modules from AIR.

### Milestone 6: parity and rollout

- [ ] Add Go target parity tests using the existing `vm_next` parity corpus.
- [ ] Run sample programs through `--target go`.
- [ ] Add project-level regression coverage for real Ard applications.
- [ ] Add runtime benchmark coverage for the rewritten Go target.
- [ ] Reach the release gate: `ard run --target go` can run all existing Ard
      programs.
