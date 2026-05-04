# AIR and vm_next Architecture

This document captures the proposed direction for moving Ard toward a stand-alone,
multi-target typed language without making Go the semantic center.

The goal is not to refactor the current bytecode VM in place. The current VM is
deeply shaped around `runtime.Object`, checker-backed runtime metadata, and
object-oriented FFI wrappers. Instead, this direction introduces a shared Ard IR
and a new VM implementation that can prove the model before a new Go target is
built from scratch around it.

## Goals

- Define a target-neutral Ard IR that becomes the shared source for future
  backends.
- Build `vm_next` from scratch against that IR without `runtime.Object`.
- Preserve Ard's syntax, testing model, multi-target support, and single-file
  executable story.
- Make FFI signature-driven and target-native by default.
- Allow the standard library to be self-hosted in Ard wherever possible.
- Leave room for a later Go target rewrite that consumes the same IR and emits
  idiomatic Go without preserving the current Go targeting code.

## Non-goals

- Do not make the VM representation the language ABI.
- Do not expose a universal boxed Ard value.
- Do not treat `Dynamic` as a castable boxed value.
- Do not force Ard closures to masquerade as native host closures in the VM.
- Do not attempt full current-VM parity before the IR shape is proven by small
  vertical slices.

## High-level plan

```text
parse AST
  -> checker validates Ard semantics
  -> AIR lowering produces typed, target-neutral IR
  -> AIR validator checks backend-facing invariants
  -> vm_next executes AIR/lowered AIR bytecode
  -> rewritten Go target later emits Go from the same AIR
```

The order of work should be:

1. Build AIR.
2. Use AIR to build `vm_next`.
   - Generate target adapters for FFI from Ard signatures.
   - Get to behavioral parity with the current VM.
3. Use AIR to rewrite the Go target from scratch.
   - Coalesce Ard stdlib FFI bindings into idiomatic generated Go where possible.
   - Keep host-specific capabilities behind explicit externs and generated
     adapters.

## AIR

AIR is a typed, target-neutral, runtime-independent representation of Ard after
checking. It is not parse AST, checker AST, bytecode, Go IR, or a VM object
model.

AIR should be structured enough for source-oriented diagnostics and simple
backend lowering, but lowered enough that backends do not rediscover semantic
facts through `checker.Type` assertions.

### Program model

Illustrative shape:

```go
type Program struct {
    Modules []Module
    Types   TypeTable
    Traits  []Trait
    Impls   []Impl
    Externs []Extern
    Tests   []Test
    Entry   FunctionID
    Script  FunctionID
}

type Module struct {
    ID        ModuleID
    Path      string
    Imports   []ModuleID
    Types     []TypeID
    Functions []FunctionID
}

type Function struct {
    ID        FunctionID
    Module    ModuleID
    Name      string
    Signature Signature
    Captures  []Capture
    Body      Block
    IsTest    bool
    IsScript  bool
}

type Signature struct {
    Params []Param
    Return TypeID
}
```

All executable AIR should use concrete `TypeID`s. Generic source declarations can
remain checker/lowering concerns, but runtime AIR should operate on specialized
signatures and concrete type layouts.

`Entry` and `Script` are separate execution roots. `Entry` points at the normal
program entrypoint, currently explicit `fn main()`, and is what `vm_next.RunEntry`
executes. `Script` points at the synthetic function generated for top-level
executable statements, and is what `vm_next.RunScript` executes for tests,
samples, and script-style programs. `RunEntry` should not fall back to a function
named `main`; entry selection belongs in AIR lowering.

### Type table

AIR owns compact type metadata for backends:

```go
type TypeKind uint8

const (
    TypeVoid TypeKind = iota
    TypeInt
    TypeFloat
    TypeBool
    TypeStr
    TypeList
    TypeMap
    TypeStruct
    TypeEnum
    TypeOption
    TypeResult
    TypeUnion
    TypeDynamic
    TypeExtern
    TypeFunction
    TypeFiber
    TypeTraitObject
)

type TypeInfo struct {
    ID   TypeID
    Kind TypeKind
    Name string

    Elem  TypeID // list, option, fiber
    Key   TypeID // map
    Value TypeID // map value or result ok
    Error TypeID // result err

    Fields   []FieldInfo
    Variants []VariantInfo
    Members  []UnionMember

    Params []TypeID
    Return TypeID
    Trait  TraitID
}
```

Struct layout must be index-based:

```go
type FieldInfo struct {
    Name  string
    Type  TypeID
    Index int
}
```

Backends should see `Request.body` as a field index, not as a runtime map lookup.
Debugging and diagnostics can resolve field names through shared metadata.

### Statements and expressions

AIR v1 should use structured statements and expressions, not SSA.

Every expression carries its `TypeID` and source span:

```go
type Expr struct {
    Kind ExprKind
    Type TypeID
    Loc  SourceSpan
}
```

Representative operations:

- constants and local loads
- `let`, assignment, block-result evaluation, break
- `if`, `while`, range and collection loops
- direct calls, extern calls, closure calls
- closure construction with explicit capture layout
- struct construction, field get/set by index
- list and map construction and operations
- enum construction and matching
- option/result construction, matching, and `try`
- union wrapping and matching
- trait upcast and trait method call
- fiber spawn/get/wait intrinsics

The AIR validator should reject unresolved IDs, bad field indexes, call
signature mismatches, illegal assignments, invalid extern signatures, and async
captures that violate the language's isolation/send rules.

## Runtime value model

`vm_next` should not import or recreate `runtime.Object`. It may have an
execution `Value`, but that value is an implementation detail of the VM, not a
language object or FFI ABI.

Illustrative VM value shape:

```go
type Value struct {
    kind Kind
    bits uint64
    ref  uint32
}
```

Heap/runtime shapes are explicit:

```go
type StructValue struct {
    Type   TypeID
    Fields []Value
}

type ListValue struct {
    Elem  TypeID
    Items []Value
}

type MapValue struct {
    Key     TypeID
    Value   TypeID
    Storage VMMap
}

type OptionValue struct {
    Some  bool
    Value Value
}

type ResultValue struct {
    Ok    bool
    Value Value
}

type UnionValue struct {
    Union TypeID
    Tag   uint32
    Value Value
}

type ExternValue struct {
    Type   TypeID
    Handle any
}

type DynamicValue struct {
    Raw any
}
```

Runtime values do not carry `checker.Type`. Generic refinement is a compile-time
or AIR-lowering concern, not a runtime mutation.

## Dynamic

`Dynamic` is not a boxed Ard value. It is opaque external or serialized data,
closer to TypeScript `unknown` or Go `any`, but without general casting.

Normal Ard code should not inspect the raw value. It should go through
self-hosted decoders and small host-provided primitives:

```ard
json::parse(Str) Dynamic!Str
decode::string(Dynamic) Str![decode::Error]
decode::int(Dynamic) Int![decode::Error]
decode::list(Dynamic, decode::Decoder<$T>) [$T]![decode::Error]
```

There should be no general `Dynamic as T` path.

On the Go host boundary, `any` is only the representation of explicit
`Dynamic`. It is not a fallback parameter type for ordinary Ard values. Host
functions that need arbitrary serialized data should declare `Dynamic`; host
resources that should not be decoded should use `extern type`.

## FFI

FFI should be signature-driven. Ard types map to target-native representations,
and generated adapters perform the boundary conversion.

For Go, the intended mapping is:

```text
Int          -> int
Float        -> float64
Bool         -> bool
Str          -> string
[T]          -> []T
[K:V]        -> map[K]V for Ard-keyable K
struct       -> generated Go struct
enum         -> generated named integer type
T?           -> ardrt.Maybe[T] by default
T!E          -> ardrt.Result[T,E] or `(T, error)` where the binding says so
Dynamic      -> opaque dynamic representation / any behind decode APIs
extern type  -> host resource handle
fn(...) ...  -> native function only in generated Go backend; callback handle in VM
```

The old shape:

```go
func(args []*runtime.Object) *runtime.Object
```

should not exist in the new architecture.

For `vm_next`, the first adapter generation step is in-memory rather than a
userland generated file. VM construction resolves each registered AIR extern into
a reusable adapter keyed by `ExternID`, validates the host function shape against
the Ard signature, and stores the conversion plan for calls. Missing bindings can
still fail lazily when called so partially implemented stdlib modules can be
loaded during the migration.

### Go FFI project layout

Go FFI should use a user-owned Go package at the Ard project root:

```text
my_project/
  ard.toml
  go.mod
  main.ard
  ffi/
    db.go
    ard.gen.go
```

The root `go.mod` is only required for projects that opt into Go FFI. Ard should
not force every project to be a Go module.

The `ffi/` directory is userland code. Users write ordinary Go host functions in
that package, and Ard generates the adapter/types file at `ffi/ard.gen.go` with
the standard generated-code header:

```go
// Code generated by ard; DO NOT EDIT.
package ffi
```

Keeping generated adapters in the same `ffi` package lets adapters call
user-written host functions directly and lets those functions accept generated
Ard-compatible Go structs without import cycles.

If Go FFI generation is requested and the project root has no `go.mod`, the CLI
should fail with a clear setup error rather than silently creating module
structure. A later init command can create the root `go.mod` and `ffi/` scaffold.

### Internal stdlib FFI package

The first version of this should be proven inside the compiler. Treat
`compiler/std_lib` as Ard's internal Ard project, with its Go host package at:

```text
compiler/std_lib/
  *.ard
  ffi/
    ard.gen.go
```

This package should use the existing compiler module at `compiler/go.mod`, not a
nested `compiler/std_lib/go.mod`. Its Go import path is:

```text
github.com/akonwi/ard/std_lib/ffi
```

That keeps the stdlib layout equivalent to a userland project while avoiding a
second Go module inside the compiler. The generated stdlib adapters should live
at `compiler/std_lib/ffi/ard.gen.go`, and handwritten stdlib host functions can
live in the same package. The current bytecode VM can keep using `compiler/ffi`
while `vm_next` starts over in `compiler/std_lib/ffi`.

As an initial migration step, `compiler/std_lib/ffi/ard.gen.go` is generated from
the Ard declarations in `compiler/std_lib/*.ard`. It should generate the
target-side contract from Ard signatures: generated structs, opaque extern handle
types, callback handle types, host function slots, and a binding registry. The
handwritten Go implementations live in the same package and fill in the host
function slots that are ready. This keeps the new path independent from the
current `compiler/ffi` package, which is part of the architecture being replaced.

### Go FFI CLI workflow

The CLI should expose a small FFI workflow:

```text
ard ffi init
ard ffi
ard ffi check
```

`ard ffi init` creates the Go FFI scaffold for the current Ard project:

- create root `go.mod` if it does not exist
- create `ffi/` if it does not exist
- generate the initial `ffi/ard.gen.go`
- never overwrite user-written files

Useful options:

```text
ard ffi init --module github.com/user/project
ard ffi init --force
```

`ard ffi` regenerates bindings only. It should read checked Ard extern
declarations, write deterministic generated code to `ffi/ard.gen.go`, and run
`gofmt` on the generated file. It should not create a `go.mod` or scaffold the
project; that belongs to `ard ffi init`.

`ard ffi check` validates the boundary for CI and local development. It should
verify that generated bindings are current, type-check or build the Go `ffi`
package, and report missing host functions, signature mismatches, unsupported
Ard types, and stale generated files.

`ard run` and `ard build` should integrate with generated FFI bindings. They can
regenerate automatically or fail when `ffi/ard.gen.go` is stale, but they should
not silently create FFI project structure.

### Generated structs

Public Ard structs used across extern boundaries should generate target
representations.

```ard
struct User {
  id: Int,
  name: Str,
  email: Str?,
}
```

Go target representation:

```go
type User struct {
    Id    int
    Name  string
    Email ardrt.Maybe[string]
}
```

Host bindings should accept generated Ard target structs by default. Host-native
structs can be supported later through explicit adapters, not implicit structural
matching.

### Opaque externs

Host values that do not fit Ard's type system should use `extern type`.

Examples:

```ard
extern type TcpListener
extern type TcpConn
extern type RawRequest
extern type DB
```

Opaque values are not serializable by default and can only be passed to externs
or Ard functions that explicitly accept that extern type.

## Closures and callbacks

Closures need different target representations:

```text
VM backend:
  Closure { fnID, captures, signature }

Go backend:
  generated native Go closure

JS backend:
  native JS function or async function
```

For the VM, arbitrary host FFI should not receive native Go closures. It should
receive typed callback handles.

Conceptual host-facing shape:

```go
type Callback2[A, B any] struct {
    Call func(A, B) (struct{}, error)
}
```

The callback adapter owns:

- conversion between host values and VM values
- isolated or scheduled VM invocation
- panic/error handling
- callback lifetime
- concurrency policy

When Ard is compiled to Go, the same AIR closure can lower to a real Go closure.
That distinction keeps the VM from pretending Ard closures are Go functions.

## Async

Async should be an Ard runtime/backend primitive, not ordinary FFI.

The public stdlib can keep ergonomic functions like:

```ard
extern type Fiber<$T>

fn start(do: fn() Void) Fiber<Void>
fn eval(do: fn() $T) Fiber<$T>
fn join(fiber: Fiber<$T>) Void
fn get(fiber: Fiber<$T>) $T
```

But AIR should lower `start`/`eval` to a fiber spawn operation rather than a
normal extern call:

```text
MakeClosure
SpawnFiber
```

Backends then implement spawn naturally:

- VM: child VM fiber or scheduler fiber
- Go: goroutine plus fiber/result handle
- JS: Promise/fiber runtime

Async capture rules should preserve the current useful restriction: spawned
closures cannot capture mutable outer state. Longer term, this should become a
`Send`-like rule:

- immutable captures are allowed
- mutable captures are rejected unless represented through safe concurrency
  primitives
- extern handles are not capturable unless the extern type is marked fiber-safe

## HTTP and self-hosted stdlib

The current `ard/http` public interface is a good target shape, but the internals
should move toward self-hosted Ard where possible.

Current interface:

```ard
struct Request { ... }
struct Response { ... }
type HandlerFn = fn(Request, mut Response)
extern fn serve(port: Int, handlers: [Str: HandlerFn]) Void!Str
```

Challenges under the new model:

- `serve` accepts closures, so in the VM it needs callback handles rather than
  normal scalar/container FFI.
- `mut Response` requires by-reference callback parameters in AIR and backend
  adapters.
- `RawRequest` and `RawResponse` should remain opaque target handles.
- `Dynamic?` request bodies blur transport bytes with unknown decoded data; a
  future `Bytes` type would make pure Ard HTTP more realistic.

Longer term, Ard can implement most of a Go `net/http`-style stack in Ard:

```ard
fn serve(port: Int, handlers: [Str: HandlerFn]) Void!Str {
  let listener = try tcp::listen(port)
  while true {
    let conn = try listener.accept()
    async::start(fn() {
      handle_connection(conn, handlers)
    })
  }
}
```

The host layer would provide low-level opaque resources:

```ard
extern type TcpListener
extern type TcpConn

extern fn listen(port: Int) TcpListener!Str
extern fn accept(listener: TcpListener) TcpConn!Str
extern fn read(conn: TcpConn, max: Int) Bytes!Str
extern fn write(conn: TcpConn, data: Bytes) Void!Str
extern fn close(conn: TcpConn) Void
```

Pure Ard code would own HTTP parsing, routing, headers, request/response structs,
middleware, decoders, response serialization, and test helpers. TLS and HTTP/2 can
remain host-provided until the lower-level pieces are mature.

## Unions

User type unions should be first-class tagged sums in AIR.

```ard
type Printable = Str | Int
```

AIR type metadata:

```go
type UnionMember struct {
    Type TypeID
    Tag  uint32
    Name string
}
```

AIR operations:

```text
UnionWrap union=Printable member=Str value=...
MatchUnion subject=... cases=[...]
```

Runtime representation:

```go
type UnionValue struct {
    Union TypeID
    Tag   uint32
    Value Value
}
```

Backends should emit generated tagged representations, not `any`, for unions
that cross backend or FFI boundaries. This keeps unions portable and avoids
target runtime ambiguity.

`Maybe` and `Result` remain explicit built-ins rather than ordinary unions so
backends can optimize `try`, matching, and common method operations directly.

## Traits

Traits also need explicit AIR representation. Split the concepts:

- trait object: runtime value of some type implementing a trait
- trait bound: compile-time generic constraint, later optimized through static
  dispatch or monomorphization

AIR tables:

```go
type Trait struct {
    ID      TraitID
    Name    string
    Methods []TraitMethod
}

type Impl struct {
    ID      ImplID
    Trait   TraitID
    ForType TypeID
    Methods []FunctionID
}
```

AIR operations:

```text
TraitUpcast value=book trait=ToString impl=BookToStringImpl
CallTrait receiver=item trait=ToString method=0
```

VM representation:

```go
type TraitObject struct {
    Trait TraitID
    Impl  ImplID
    Value Value
}
```

Concrete method calls should remain direct when the receiver type is statically
known. Dynamic dispatch should only be paid at trait-typed boundaries.

Trait objects should not cross FFI initially. If a host type should implement an
Ard trait, model it as an `extern type` plus an Ard trait impl whose methods are
extern-backed.

## Standard library model

The stdlib should be split by capability:

- self-hosted Ard modules for data structures, decoders, result/maybe helpers,
  routing, tests, and pure protocol logic
- small target externs for privileged host capabilities like clock, filesystem,
  network, process/env, crypto primitives, JSON parse/stringify to `Dynamic`,
  and low-level server/socket hooks

Rule of thumb:

> If it can be expressed in Ard without privileged host access, it belongs in Ard.

The rewritten Go target can then coalesce Ard stdlib and extern bindings into
idiomatic generated Go. For example, pure Ard modules become generated Go from
AIR, while low-level externs become direct Go calls or generated adapters.

## Milestones

Status markers:

- `Done`: the milestone's checklist is complete.
- `In progress`: at least one item is complete, but the milestone is not done.
- `Pending`: no committed implementation work yet.

### Milestone 1: AIR skeleton

Status: Done

- [x] Define AIR program, type, function, statement, and expression data
  structures.
- [x] Lower a tiny checked program to AIR.
- [x] Add an AIR validator.
- [x] Ensure AIR does not import `runtime.Object` or expose `checker.Type` to
  backend execution.

### Milestone 2: vm_next scalar execution

Status: Done

- [x] Execute constants, locals, arithmetic, direct function calls,
  conditionals, and block-result evaluation.
- [x] Add minimal test harness integration for AIR programs.
- [x] Support `ard run --target vm_next` for the scalar subset.

### Milestone 3: layouts and generated data

Status: Done

- [x] Add struct layout metadata.
- [x] Execute struct construction and field get/set by index.
- [x] Execute enum construction, equality, and matching.
- [x] Execute `Maybe` constructors, equality, matching, and non-closure helper
  operations.
- [x] Execute `Result` matching and non-closure helper operations beyond
  `ok`/`err` constructors.
- [x] Add generated Go representation for Ard structs used by test FFI
  examples.

### Milestone 4: try and control flow

Status: Done

- [x] Lower `try` explicitly in AIR instead of treating it as a backend-local
  trick.
- [x] Execute `try` for `Maybe` and `Result`, including catch handlers and
  propagation through expression blocks.
- [x] Add bool-match lowering and execution needed by self-hosted helpers like
  `ard/testing::assert`.
- [x] Run the built-in testing helpers through `vm_next` without depending on the
  current VM once self-hosted stdlib module lowering is available.
- [x] Validate that `try` preserves Ard's expression-return semantics in nested
  blocks, while loops, and match arms.

### Milestone 5: FFI adapters

Status: Done

- [x] Generate VM FFI adapters from Ard signatures.
- [x] Establish `compiler/std_lib/ffi` as the internal stdlib Go FFI package and
  generate `compiler/std_lib/ffi/ard.gen.go` from Ard extern declarations.
- [x] Execute scalar, generated `Maybe[T]`, and error-backed `Result` externs
  in `vm_next` through the generated stdlib host registry.
- [x] Convert AIR structs to and from native Go structs for `vm_next` extern
  calls.
- [x] Carry opaque extern handles across `vm_next` extern calls without exposing
  them as `Dynamic` or boxed runtime objects.
- [x] Support scalar parameters and returns.
- [x] Support generated structs across the VM FFI boundary.
- [x] Support generated `Maybe[T]` and error-backed `Result` values.
- [x] Support opaque extern handles.
- [x] Support maps/lists of representable values.
- [x] Keep raw escape hatches out of the default path.

### Milestone 6: closures and async

Status: Done

- [x] Add closure values and capture layout.
- [x] Execute closure-based `Maybe` helpers such as `map` and `and_then`.
- [x] Execute closure-based `Result` helpers such as `map`, `map_err`, and
  `and_then`.
- [x] Add fiber spawn/get/wait as AIR intrinsics.
- [x] Enforce async capture isolation.
- [x] Add typed callback handles for VM-to-host callback APIs.

### Milestone 7: complicated types

Status: Done

- [x] Add user unions as tagged sums.
- [x] Add trait and impl tables.
- [x] Add trait objects with explicit upcast and method dispatch.

### Milestone 8: vm_next parity

Status: Done

- [x] Separate program entry execution from script execution with
  `vm_next.RunEntry` and `vm_next.RunScript`.
- [x] Run current VM behavioral tests against `vm_next`.
  - [x] Add `vm_next` parity coverage for the current bytecode VM's pure
    language/runtime regression cases: expressions, block expressions, function
    calls, nullable argument wrapping, `try`, `Maybe`, `Result`, lists, maps,
    loops, matching, structs, enums, unions, generic equality, type helper
    methods, concurrent runtime access, and Maybe propagation through `try`.
  - [x] Close parity gaps exposed by those tests: AIR block expressions,
    C-style `for` loops, string iteration, conditional matches, sorted map
    iteration, checker `Identifier` locals in optional property chains, and
    concrete `Void` layouts for uninhabited generic values like `none` and
    error-only `Result` expressions.
  - [x] Port bytecode VM host-capability tests where they apply to the new FFI
    model.
    - [x] Port duration, `Dynamic`/decode/from_json, env, filesystem, UUID,
      FFI panic recovery, IO printing, HTTP client timeout behavior, and async
      timing coverage.
    - [x] Restore successful `decode::path` payload cases after nested generic
      decoder factories preserve returned payload types through closure
      specialization.
    - [x] Port SQL coverage for parameter extraction, query execution, null
      values, missing parameter errors, and transactions.
    - [x] Port JSON encode coverage for primitive `Encodable` values.
    - [x] Port broader stdlib crypto coverage for hash digests, password
      verification, and scrypt helpers.
    - [x] Port server-side HTTP callback behavior for route maps, mutable
      responses, request bodies, and raw request opaques.
  - [x] Decide which bytecode VM internal tests are intentionally not copied
    because they validate old registry/type-resolver/profile implementation
    details rather than Ard runtime behavior.
    - The old FFI registry registration/generation tests are not copied because
      `vm_next` uses AIR extern signatures plus generated host adapters instead
      of the bytecode `runtime.Object` registry. The behavior-facing panic
      recovery case is retained as parity coverage.
    - The old string type resolver test is not copied because AIR carries typed
      IDs and signatures directly instead of parsing type names at runtime.
    - The old execution profile report test is not copied because it validates a
      bytecode VM profiling surface that `vm_next` has not adopted.
    - Method/module registry concurrency risks are covered through concurrent
      `vm_next` runtime parity tests rather than by preserving the old cache
      implementation.
- [x] Support `ard run --target vm_next` for all `vm_next`-targetable sample
  programs.
  - Core samples cover template strings, list/map methods, list/map iteration,
    imported module externs, string helpers, EOF-driven stdin, and generic
    stdlib function specialization for `ard/async::join`.
  - Interactive samples run with controlled stdin.
  - `samples/pokemon.ard` runs through HTTP client FFI, `Dynamic` JSON parsing,
    generated `Result[T,E]` decode primitives, module function references, and
    generic decoder specialization.
  - `samples/server.ard` starts under `vm_next`; verified routes exercise
    VM-to-host callback handles, mutable response parameters, inbound request
    opaques, and JSON request decoding.
  - `samples/pokemon_js.ard` remains intentionally target-gated to JS modules
    and is not a `vm_next` sample.
- [x] Add `vm_next` to the benchmark suite.
  - `vm_next` participates in CLI-mode benchmarks through
    `ard run --target vm_next`; runtime-mode binary comparisons remain
    bytecode/Go/JS/native-Go until `vm_next` has a buildable executable artifact.
- [x] Do not add a separate conformance harness for this milestone.
  - Coverage comes from the `vm_next` parity tests, `ard run --target vm_next`
    sample execution, and CLI benchmark participation.
  - Later Go target rewrite work can reuse those same samples and parity cases
    instead of introducing another parallel suite now.

### Post-Milestone 8: userland FFI workflow

Moved to [userland-ffi-workflow.md](userland-ffi-workflow.md).

Status: Pending

### Milestone 9: vm_next executable builds

Status: Done

- [x] Support `ard build --target vm_next` as a real build target.
- [x] Produce single-file executables for `vm_next` programs so the new VM can
  be exercised with the same portability story as the current bytecode VM.
- [x] Define a serialized AIR or lowered-VM artifact format that is explicitly a
  `vm_next` implementation detail, not the language ABI.
- [x] Teach embedded executable startup to dispatch to `vm_next.RunEntry`.
- [x] Add build/run verification for generated `vm_next` executables through
  focused tests and benchmark-runner output checks.
- [x] Add `vm_next` executables to runtime-mode benchmarks once build support
  exists.
- [x] Consider Milestone 8 plus this milestone the parity gate for replacing the
  current bytecode VM as the primary VM target.

### Post-Milestone 9: vm_next execution performance

Completed. The final record lives in
[`compiler/docs/vm-next-execution-performance.md`](../compiler/docs/vm-next-execution-performance.md).

Status: Done

The closed execution-performance work covered the real `AIR -> vm_next`
bytecode/lowered instruction layer plus runtime optimizations for frames,
argument passing, value representation, generated FFI adapters, collections,
closures, traits, async, and profiling.

### Milestone 10: rewrite the Go target

Moved to [rewrite-go-target.md](rewrite-go-target.md).

Status: Pending
