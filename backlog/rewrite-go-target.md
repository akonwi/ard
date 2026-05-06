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
- [x] Add basic import planning for the current subset.
- [x] Define the extern lowering model in code for the current subset.
  - direct lowering now exists for the current `go = "Print"` path used by
    `ard/io::_print`

### Milestone 2: structured control flow

Status: Complete

- [x] Lower block expressions through temporaries and statement setup for the
      current scalar subset.
- [x] Lower `if` expressions and `if` statements for the current scalar subset.
- [x] Lower assignments and mutable locals for the current scalar subset.
- [x] Lower loops and control-flow-heavy statement bodies for the current
      scalar subset.
- [x] Expand control-flow lowering enough to run the current loop-oriented
      sample subset (`samples/loops.ard`).
- [x] Expand control-flow lowering enough to support Maybe-based branching in
      the current sample subset (`samples/nullables.ard`).
- [x] Expand control-flow lowering enough to support enum matching in the
      current sample subset (`samples/lights.ard`).
- [x] Expand control-flow lowering coverage beyond the current sample subset.
  - current control-flow coverage now reaches interactive/result-heavy and
    decode-heavy programs such as `todo-list`, `pokemon`, and buildable server
    flows
- [x] Lower `try` into explicit intermediate values and early returns.
  - current lowering handles `try` for `Maybe` and `Result`, with catch blocks
    and propagated early returns across differing result value shapes

### Milestone 3: core data model

Status: Complete

- [x] Generate Ard structs as Go structs for the current subset.
  - basic struct field write lowering now exists through `StmtSetField`
- [x] Generate enums for the current subset.
- [x] Lower lists and maps to native Go containers where representable.
  - current subset support includes list literals plus `size`, `at`, `push`,
    `prepend`, `set`, `swap`, and sort-backed comparator flows
  - current subset support includes map literals plus `size`, `has`, `get`,
    `set`, `drop`, `keys`, key iteration helpers, and value iteration helpers
  - this is enough to run `samples/collections.ard`, `samples/maps.ard`, and
    decode-driven map/list workflows such as `pokemon`
- [x] Settle the default Go lowering for `Maybe`.
  - the backend now treats `runtime.Maybe[T]` from
    `github.com/akonwi/ard/runtime` as the default Go representation for Ard
    nullable values
- [x] Settle the default Go lowering for `Result`.
  - the backend now treats `runtime.Result[T, E]` from
    `github.com/akonwi/ard/runtime` as the default Go representation for Ard
    result values
- [x] Keep runtime helpers minimal and justify each one.
  - the shared helper surface is intentionally limited to shared runtime types
    like `runtime.Maybe[T]` and `runtime.Result[T, E]`, generated helpers such
    as `ardFiber`, stdin parsing helpers, and deterministic key/dynamic
    conversion helpers needed by generated container and host flows
- [x] Expand import planning beyond the current minimal subset.
  - imports are now emitted from actual alias usage in generated AST instead of
    eagerly mirroring the full runtime prelude surface

### Milestone 4: advanced language features

Status: Complete

- [x] Generate tagged unions.
  - current subset support uses generated union structs with a tag field plus
    one field per member type
  - this is enough to run `samples/type-unions.ard`
- [x] Generate native Go closures and capture handling.
  - current subset support exists for generated closure literals used in sample
    comparator flows such as `List.sort`
  - current subset support also covers captured closures in async sample flows
- [x] Lower trait dispatch where dynamic trait objects are required.
  - current subset support covers dynamic `Str::ToString` dispatch used by
    `samples/traits.ard`
- [x] Lower fibers to goroutines and typed handles.
  - current subset support exists through generated `ardFiber[T]` helpers and
    `ardSpawnFiber` / `ardJoinFiber` / `ardGetFiber`
  - this is enough to run `samples/concurrent_stress.ard`

### Milestone 5: FFI and stdlib integration

Status: Complete

Open design questions to settle during this milestone:
- [x] Always lower externs to direct static Go calls.
  - generated Go should call the Ard Go FFI surface directly, with no extra
    backend-generated wrapper layer
  - the extern binding signature tells the backend how to interpret returned Go
    values and lower them into canonical generated-Go shapes such as
    `runtime.Result[...]` and `runtime.Maybe[...]`
  - host FFI functions may still return native Go tuples where useful, but
    generated Go should consistently use the Ard runtime representations
  - if a boundary is awkward to call directly, the FFI surface should be
    normalized rather than patched with backend-generated adapters
- [x] Settle the default Go lowering for `Dynamic`.
  - the backend now treats `Dynamic` as plain Go `any`
  - `Dynamic` is specifically for arbitrary data at I/O boundaries such as API
    payloads and decoded input data, with explicit parsing done through the
    decode API
  - preserving non-Ard host values across Go/Ard boundaries should prefer
    opaque types rather than treating `Dynamic` as a general extern transport
- [x] Do not emit general-purpose list/map/object conversion helpers for
      `Dynamic`.
  - `Dynamic` lowers to Go `any`
  - typed interpretation of dynamic I/O data belongs to explicit APIs,
    especially `ard/decode`
  - `ard/decode` acts as the JSON/dynamic FFI surface via a small explicit host
    API: JSON parsing into `any`, scalar decoders returning structured decode
    errors, and structural helpers such as list/map/field extraction and nil
    checks
  - any remaining conversion code should be limited to unavoidable host-API
    marshaling, not general `Dynamic` lowering semantics
- [x] Represent opaque host resource handles as Go `any`.
  - generated Go should treat opaque extern values as fully opaque and only
    store, pass, and return them across the FFI boundary
  - host FFI implementations are responsible for asserting the expected
    concrete Go types
- [x] Lower callback-shaped externs as anonymous Go funcs/closures passed
      directly into static FFI calls.
  - callback signatures should follow the normal Ard-to-Go lowering rules:
    mutable params become pointers where needed, `Dynamic` lowers to `any`,
    opaque handles lower to `any`, and optional/fallible returns use
    `runtime.Maybe[...]` / `runtime.Result[...]`
  - if a host API expects a different callback convention, that normalization
    belongs in the FFI implementation rather than backend-generated adapters

- [x] Lower extern calls to direct Go calls by default.
  - current subset support exists for `Print`, `FloatFromInt`, `FloatFromStr`,
    `FloatFloor`, `ReadLine`, `IntFromStr`, `Sleep`, `EnvGet`, `OsArgs`,
    base64/hex helpers, JSON/decode helpers, and HTTP client response flows;
    broader extern coverage is still pending
  - this is enough for the current `ard/io::_print` path, simple numeric
    conversion cases, initial stdin-backed string input flows, async sleep
    flows, common encoding/env/argv helpers, and HTTP client / decode-driven
    samples like `pokemon`
- [x] Keep backend-generated extern adapters exceptional and minimize their
      remaining surface.
  - the backend now lowers most externs as direct static calls into the Ard Go
    FFI surface, including `ReadLine`, `IntFromStr`, `FloatFromStr`,
    `Base64*`, `EnvGet`, `Sleep`, dynamic constructors, and HTTP serve/client
    entry points
  - remaining backend-side adaptation is intentionally narrow and currently
    limited to cases such as decode error-struct bridging and dynamic map key
    marshaling
- [x] Support generated structs across the Go extern boundary.
  - stdlib-facing named structs/enums now reuse the generated Go FFI surface
    where appropriate, and HTTP request/response flows build and run through
    generated Go with direct static FFI entry points
- [x] Support opaque extern types.
  - opaque extern host types now lower as `any`, with host FFI code
    responsible for concrete type assertions
- [x] Support callback externs as native Go closures.
  - callback-shaped FFI signatures now lower through native Go func types,
    including HTTP serve handlers
  - mutable struct params in the generated Go subset lower through pointer
    parameters/calls where needed to preserve callback-side mutations
- [x] Compile self-hosted stdlib modules from AIR.
  - current lowering compiles deeper self-hosted stdlib paths such as
    `ard/decode`, plus common extern-backed utility modules like `ard/base64`,
    `ard/hex`, `ard/env`, `ard/argv`, `ard/float`, and `ard/dynamic`
  - root Go target coverage now includes server/decode-oriented flows that
    exercise these modules through generated Go builds/tests

### Milestone 6: parity and rollout

- [ ] Add Go target parity tests using the existing `vm_next` parity corpus.
  - current Go-target-vs-vm_next parity coverage now exercises a broader core
    subset via generated-program JSON result comparison, including arithmetic,
    comparison chains, string size checks and string helper methods,
    structured branching, recursion, closures, anonymous-function inference,
    list sorting, sorted map keys, loop forms over ranges/numbers/strings/
    lists/maps plus break behavior, list/map mutation and access operations,
    nullable-argument omission and auto-wrapping flows, nullable struct field
    omission and auto-wrapping, boolean/enum/int/range/conditional match
    forms, enum/int comparisons and explicit enum values, union matching,
    generic equality cases, Result try propagation/catch flows including nested
    `try` inside match arms returning `Result` values, Maybe try basics,
    Maybe/Result matches, Maybe/Result fallback and predicate combinators,
    callback-based Maybe/Result map/and_then/map_err combinators, env lookup,
    JSON primitive encoding, crypto hash externs, and a broader decode-host
    subset including dynamic list/object conversion, `decode::from_json`,
    `decode::field`, `decode::path`, decode error reporting, nullable decode,
    list/map decode, `decode::one_of`, and `decode::flatten`
  - Go target host/decode parity now also covers the `IsNil` extern path used
    by `decode::nullable(...)`, direct `panic(...)` lowering used by decode
    error assertions in the parity corpus, and crypto hash extern bindings
  - Go target host parity now also covers the `ard/fs` extern surface needed by
    current parity corpus slices: exists/is_file/is_dir, create/write/append/
    read/delete/copy/rename, cwd/abs, create/delete dir, and list_dir
  - Go target host parity also now covers `crypto::uuid()`, HTTP method/client
    timeout parity, and the current SQL parity slice including parameter
    extraction, query all/first, missing-parameter errors, nullable SQL values,
    rollback, and commit/transaction query flows
  - async parity is now promoted for current vm_next timing/value coverage:
    `async::sleep`, `async::start(...).join()`, and `async::eval(...).get()`
  - next likely parity gap is no longer language/runtime async itself, but the
    lack of a Go-target parity harness path for custom extern overrides used by
    vm_next tests like FFI panic recovery and HTTP server callback interception;
    generated Go currently runs against the concrete stdlib host directly
  - parity harness result normalization now unwraps generated top-level
    `runtime.Maybe`/`runtime.Result` values to match vm_next observable output
- [ ] Run sample programs through `--target go`.
  - current direct run parity against the default bytecode target now matches
    for the non-interactive sample corpus:
    `variables`, `collections`, `fibonacci`, `fizzbuzz`, `grades`, `lights`,
    `loops`, `maps`, `maths` (module-only/no entry), `modules`, `nullables`,
    `temperatures`, `traits`, `type-unions`, `concurrent_stress`, `pokemon`,
    `word_frequency`, and `escape-sequences`
  - current build coverage now also includes interactive/server-oriented sample
    binaries: `guess`, `todo-list`, `tic-tac-toe`, and `server`
  - current end-to-end interactive checks now match between default bytecode and
    `--target go` for `guess`, `todo-list`, and `tic-tac-toe` with scripted
    stdin flows
  - manual route checks now cover `/`, `/me`, `/error`, and
    `/api/auth/sign-up`; Go-target server responses match the default runtime
    semantically (status/body/headers except expected transport-varying fields
    like `Date`)
  - current next sample-observability work is broader server-side/manual route
    coverage beyond the checked paths, rather than core sample execution parity
  - current manual sample coverage also exercises generated closure literals via
    `List.sort` in `tic-tac-toe`
- [ ] Add project-level regression coverage for real Ard applications.
- [ ] Preserve generated target artifacts under a project-local `.build/`
      directory for inspection and debugging.
- [ ] Add runtime benchmark coverage for the rewritten Go target.
- [ ] Reach the release gate: `ard run --target go` can run all existing Ard
      programs.
