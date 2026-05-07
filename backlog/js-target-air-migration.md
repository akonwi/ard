# JavaScript Target AIR Migration

This backlog tracks rewriting Ard's JavaScript targets (`js-server` and
`js-browser`) to consume AIR instead of checker nodes directly.

## Status

In progress

## Context

Recent compiler work introduced AIR as the typed backend-facing representation
between checking and code generation/runtime execution. The Go target has already
been rewritten around this architecture and now serves as the reference codegen
backend shape.

The current JavaScript target still lives under `compiler/javascript` and lowers
from `checker.Module` / checker expression nodes. Its public API also owns
frontend loading:

- `Build(inputPath, outputPath, target)`
- `Run(inputPath, target, args)`

This means the JS target bypasses the new backend boundary used by `vm` and the
Go target:

```text
Ard source
→ parse/frontend
→ checker
→ AIR
→ backend
```

The migration should make JavaScript a first-class AIR backend while preserving
current target behavior, prelude/runtime helpers, and FFI companion support.

## Goals

- Make the JavaScript backend consume `*air.Program` as its semantic input.
- Move frontend loading and AIR lowering out of `compiler/javascript` and into
  the shared CLI flow, mirroring the Go target.
- Preserve both JavaScript targets:
  - `js-server`: runnable through Node.
  - `js-browser`: build-only ES module output.
- Preserve current generated module behavior where practical:
  - one emitted `.mjs` file per Ard module
  - prelude copied as `ard.prelude.mjs`
  - stdlib/project FFI companions copied when needed
- Keep JavaScript representation pragmatic and native-first:
  - numbers for `Int`/`Float`
  - strings/bools as native JS primitives
  - arrays for lists
  - `Map` for maps
  - classes or object records for structs
  - prelude-backed `Maybe`, `Result`, enum, equality, and error helpers
- Add AIR validation before JS lowering.
- Use tests to drive parity and avoid silent regressions.

## Non-goals

- Do not preserve checker-node lowering as a long-term backend path.
- Do not introduce a VM-shaped JavaScript object runtime.
- Do not model all Ard values as boxed dynamic values.
- Do not redesign JavaScript FFI from scratch in the first migration pass.
- Do not attempt to make `js-browser` runnable from the CLI.
- Do not require a full JavaScript AST dependency unless string/doc emission
  becomes unmaintainable.

## Target Architecture

The intended JavaScript backend surface should mirror the Go target's AIR-first
shape:

```go
type Options struct {
    Target string // backend.TargetJSServer or backend.TargetJSBrowser
    RootFileName string
    InvokeMain bool
}

func GenerateSources(program *air.Program, options Options) (map[string][]byte, FFIArtifacts, error)
func RunProgram(program *air.Program, target string, args []string) error
func BuildProgram(program *air.Program, outputPath string, target string, projectInfo *checker.ProjectInfo) (string, error)
```

Exact signatures may change during implementation, but the important boundary is
that code generation takes AIR, not checker nodes or source paths.

The CLI should become responsible for:

1. frontend loading with the requested target
2. checker diagnostics
3. `air.Lower(module)`
4. dispatching the AIR program to the selected backend

## Current JavaScript Backend Inventory

Files and responsibilities to account for during migration:

- `compiler/javascript/javascript.go`
  - current checker-node emitter
  - build/run orchestration
  - module import planning
  - FFI companion detection/copying
  - expression/statement lowering
- `compiler/javascript/ir.go`
  - lightweight JS expression/statement IR used by the emitter
- `compiler/javascript/docs.go`
  - pretty-doc helpers for JS source rendering
- `compiler/javascript/ard.prelude.mjs`
  - runtime helpers for `Maybe`, `Result`, enums, equality, errors, and break
    signaling
- `compiler/std_lib/ffi.js-server.mjs`
- `compiler/std_lib/ffi.js-browser.mjs`
- `compiler/javascript/javascript_test.go`
  - existing behavior and generated-output tests

## Design Notes

### AIR input

The new JS lowerer should call `air.Validate(program)` before lowering. Backend
logic should use AIR type/function/module IDs and `air.TypeInfo` instead of
checker type assertions.

### Root execution

Use AIR roots instead of guessing source-level names:

- `program.Entry` for `fn main()`
- `program.Script` for top-level executable statements

`js-server` run/build should emit an async-compatible root invocation where
needed. `js-browser` should build the module without trying to execute it from
CLI.

### Module output

Preserve inspectable module output:

```text
main.mjs
ard.prelude.mjs
ffi.stdlib.js-server.mjs      # when used
ffi.project.js-server.mjs     # when used
<imported-module>.mjs         # for emitted Ard modules
```

The exact module file naming can reuse the existing `moduleOutputPath` helpers
if they remain suitable.

### Runtime/value representation

Initial AIR lowering should keep the current JavaScript representation unless a
specific AIR construct requires adjustment:

```text
Void        -> undefined
Int         -> number, integer semantics where needed
Float       -> number
Bool        -> boolean
Str         -> string
[T]         -> Array<T>
[K:V]       -> Map<K,V>
struct      -> generated class or equivalent record
unique enum -> prelude enum object/value shape
union       -> tagged prelude/object shape, or native value plus tag metadata
T?          -> Maybe
T!E         -> Result
Dynamic     -> any JS value
extern type -> opaque JS value
fn          -> JS function
Fiber[T]    -> prelude/helper-backed promise/task shape if supported
trait object -> pragmatic dispatch shape, matching current AIR metadata needs
```

### Extern/FFI

Keep existing JS extern companion behavior at first:

- stdlib extern JS implementations are copied from `compiler/std_lib`.
- project extern JS implementations are copied from project root companions.
- bindings select by `extern.Bindings[target]`.
- common `js` fallback bindings should remain supported if still emitted by AIR.

The old checker-based FFI artifact scan needs an AIR equivalent over
`program.Externs` and module metadata.

### Rendering

The migration can continue using the existing lightweight JS IR and pretty-doc
renderer. A full JS AST is not required for the first AIR rewrite.

However, AIR lowering should be separated from orchestration so the backend has
clear stages:

```text
air.Program
→ JS lowering state / module files
→ JS doc/string rendering
→ emitted .mjs sources
→ optional companion copy
→ optional node execution
```

## Implementation Phases

### Phase 1: Backend API and orchestration skeleton

Create the AIR-first backend surface while keeping the old implementation
available temporarily for comparison.

- Add AIR-oriented `Options` and source-generation entrypoint.
- Add `RunProgram` and `BuildProgram` variants that accept `*air.Program`.
- Keep FFI companion copying reusable.
- Update or prepare CLI call sites to be switched once parity is ready.

### Phase 2: Minimal AIR source emission

Implement enough AIR lowering to generate and run simple programs:

- module preamble/imports
- root function/script selection
- primitive constants
- local loads
- `let` and assignment
- function declarations and direct calls
- arithmetic/comparison/boolean operations
- string concatenation and `to_str`
- expression statements and returns

### Phase 3: Core data types

Port current JS representations to AIR metadata:

- lists and list methods
- maps and map methods
- structs and field access/set
- enums and enum matching
- maybe/result construction and methods
- equality semantics using `ardEq` where needed

### Phase 4: Control flow and expression lowering

Port statement/expression lowering that currently relies on checker nodes:

- block expressions
- `if` expressions
- `while`
- break handling
- AIR match forms:
  - enum
  - int/range
  - union
  - maybe
  - result
- `try` for result/maybe
- panic/error emission

### Phase 5: Modules, externs, and browser/server behavior

Complete multi-module and host-boundary support:

- imported Ard module source generation
- deterministic module aliases/file names
- extern function wrappers from `air.Extern`
- stdlib/project FFI companion detection from AIR
- `js-server` Node run flow
- `js-browser` build-only flow

### Phase 6: Advanced AIR constructs

Add or explicitly defer support for remaining AIR constructs:

- closures and captured locals
- function values / closure calls
- traits and trait calls
- union wrapping and matching details
- dynamic conversions
- fibers / async intrinsics
- tests, if JS test execution becomes desired

### Phase 7: CLI switch and cleanup

Once parity is acceptable:

- switch `main.go` JS run/build paths to frontend → checker → AIR → JS backend
- remove old source-path/checker-node JS entrypoints
- delete dead checker-node lowering helpers
- update docs/backlog status
- run broad test coverage

## Checklist

### Planning and scaffolding

- [ ] Confirm desired public JS backend API names/signatures.
- [x] Add AIR-first backend options type.
- [x] Add `GenerateSources` for `*air.Program`.
- [x] Add `RunProgram` for `*air.Program`.
- [x] Add `BuildProgram` for `*air.Program`.
- [x] Ensure JS lowering calls `air.Validate`.
- [x] Keep old checker backend callable during incremental migration.

### CLI integration

- [ ] Update `ard run --target js-server` to lower AIR before JS dispatch.
- [ ] Keep `ard run --target js-browser` rejected with a clear message.
- [ ] Update `ard build --target js-server` to lower AIR before JS dispatch.
- [ ] Update `ard build --target js-browser` to lower AIR before JS dispatch.
- [x] Preserve project-info access for project FFI companion copying.

### Module/source generation

- [x] Emit prelude imports for generated module files.
- [x] Emit one `.mjs` file per AIR module.
- [x] Generate deterministic module aliases.
- [x] Generate deterministic output paths for imported modules.
- [x] Export public module symbols where needed.
- [x] Invoke AIR root function/script for `js-server` run output.

### Type declarations and values

- [x] Lower primitive AIR types.
- [x] Lower struct type declarations.
- [x] Lower struct construction.
- [x] Lower struct field access.
- [x] Lower struct field assignment.
- [x] Lower enum declarations.
- [x] Lower enum variants.
- [x] Lower union wrapping.
- [x] Lower maybe values.
- [x] Lower result values.
- [x] Lower list values.
- [x] Lower map values.
- [x] Lower dynamic/extern opaque values.

### Functions and locals

- [x] Lower AIR function declarations.
- [x] Lower parameters and local IDs to stable JS names.
- [x] Lower local loads.
- [x] Lower `let` statements.
- [x] Lower assignments.
- [x] Lower direct function calls.
- [x] Lower extern calls.
- [x] Lower closures.
- [x] Lower closure calls.
- [x] Lower captures.

### Expressions and operators

- [x] Lower int arithmetic.
- [x] Lower float arithmetic.
- [x] Lower string concatenation.
- [x] Lower comparisons.
- [x] Lower equality / inequality.
- [x] Lower boolean `and` / `or` / `not`.
- [x] Lower numeric negation.
- [x] Lower `to_str`.
- [x] Lower copy expressions.
- [x] Lower panic expressions.

### Collections

- [x] Lower list `at`.
- [x] Lower list `prepend`.
- [x] Lower list `push`.
- [x] Lower list `set`.
- [x] Lower list `size`.
- [x] Lower list `sort`.
- [x] Lower list `swap`.
- [x] Lower map `keys`.
- [x] Lower map `size`.
- [x] Lower map `get`.
- [x] Lower map `set`.
- [x] Lower map `drop`.
- [x] Lower map `has`.
- [x] Lower map key/value indexed iteration helpers if needed by AIR loops.

### Control flow and matching

- [x] Lower block expressions.
- [x] Lower `if` expressions.
- [x] Lower `while` statements.
- [x] Lower break handling.
- [x] Lower enum match.
- [x] Lower int/range match.
- [x] Lower union match.
- [x] Lower maybe match.
- [x] Lower result match.
- [x] Lower result `try`.
- [x] Lower maybe `try`.

### Maybe/result methods

- [x] Lower maybe `expect`.
- [x] Lower maybe `is_none`.
- [x] Lower maybe `is_some`.
- [x] Lower maybe `or`.
- [x] Lower maybe `map`.
- [x] Lower maybe `and_then`.
- [x] Lower result `expect`.
- [x] Lower result `or`.
- [x] Lower result `is_ok`.
- [x] Lower result `is_err`.
- [x] Lower result `map`.
- [x] Lower result `map_err`.
- [x] Lower result `and_then`.

### Traits, dynamic, and async

- [x] Lower trait upcasts.
- [x] Lower trait method calls.
- [x] Lower `Dynamic` conversions.
- [ ] Lower spawn fiber.
- [ ] Lower fiber get.
- [ ] Lower fiber join.
- [ ] Decide whether any async JS functions must be emitted as `async`.

### Externs and FFI companions

- [x] Detect stdlib JS FFI usage from AIR extern metadata.
- [x] Detect project JS FFI usage from AIR extern metadata.
- [x] Preserve `js` binding fallback if applicable.
- [x] Preserve `js-server` binding selection.
- [x] Preserve `js-browser` binding selection.
- [x] Copy `ard.prelude.mjs`.
- [x] Copy stdlib FFI companion when used.
- [x] Copy project FFI companion when used.
- [x] Preserve external return adaptation for `Maybe`.
- [x] Preserve external return adaptation for `Result`.
- [ ] Preserve external struct/list/map adaptation where currently supported.

### Tests and validation

- [x] Add unit tests for minimal AIR JS source generation.
- [ ] Port existing JS build tests to the AIR backend.
- [x] Add `js-server` run parity tests for primitive programs.
- [ ] Add parity tests for structs/enums/lists/maps.
  - [x] Added initial AIR source assertions for list/map values and methods.
  - [x] Added initial AIR source assertions for union wrap/match.
- [ ] Add parity tests for maybe/result/try/match.
  - [x] Added initial AIR source assertions for Maybe/Result constructors and `or`.
  - [x] Added initial AIR source assertions for enum/int/maybe/result matches.
- [ ] Add parity tests for modules/imports.
  - [x] Added initial AIR source assertions for imported function calls.
- [ ] Add parity tests for JS externs.
  - [x] Added initial AIR source assertions for project extern imports and Maybe/Result return adapters.
- [ ] Keep or update browser-output tests.
- [ ] Run `cd compiler && go test ./javascript`.
- [ ] Run `cd compiler && go test ./air ./go ./backend`.
- [ ] Run `cd compiler && go generate ./std_lib/ffi && go test ./...` before final cleanup.

## Open Design Points

- Whether generated JS should remain string/doc based or move to a more formal
  JS AST representation later.
- Whether `Maybe`/`Result` should stay prelude classes or eventually become
  simpler generated object shapes.
- How much of JS async/fiber support should be implemented in the first AIR
  migration pass.
- Whether JS output should preserve existing class-based structs or move to
  object literals with helper constructors.
- Whether browser builds need a dedicated output directory convention like
  `ard-out/js-browser/build` after the AIR rewrite.
- How to expose generated JS artifacts for inspection during `ard run`, similar
  to the Go target's `ard-out/go/run` workspace.

## Progress Notes

- Created on branch `refactor.js-target` as the migration reference checklist.
- Added initial AIR-first JavaScript backend scaffold in `compiler/javascript/air_backend.go`.
- Added minimal AIR source-generation/build tests while keeping the legacy checker backend intact.
- Fixed AIR TypeID indexing in the JS lowerer and expanded AIR lowering for strings, collections, Maybe, and Result operations.
- Added AIR imported-function module dependency detection, basic extern call references, and struct field assignment lowering.
- Added AIR enum/int/maybe/result match expression lowering using JavaScript IIFEs.
- Added a primitive `js-server` AIR `RunProgram` smoke test.
- Added AIR dynamic identity lowering and map key/value indexed helper lowering used by map iteration.
- Added AIR closure/capture lowering and simple tagged-object union wrap/match lowering.
- Added AIR try lowering for tail expressions and `let` bindings with Result/Maybe propagation.
- Added AIR ToString trait call lowering and Maybe/Result extern return adapters.
