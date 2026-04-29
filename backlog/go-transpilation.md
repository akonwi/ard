# Go Backend Future Work

This document is a backlog/proposal document for future work on Ard's Go backend.

It is **not** the source of truth for the current implementation. For the current production architecture, see:

- `compiler/docs/go-emission-architecture.md`

The Go backend itself now lives in:

- `compiler/go_backend/`

This file should only capture follow-up ideas, open design questions, and future implementation directions.

## Goals for future work

Potential future investment areas for the Go backend include:

- reducing end-to-end Go-target build times
- expanding backend optimization passes
- improving generated Go compile-time characteristics
- growing parity coverage between the bytecode VM and Go backend
- improving backend-local testing and benchmarking
- refining packaging/layout for generated projects and stdlib emission
- extending the Go backend to support more workflows, especially test mode

## Long-term vision

The long-term goal for Ard's Go target is for it to behave like a real host-language backend, not a runtime-hosted compatibility layer.

In practical terms, the end goal is:

- Ard transpiles cleanly to ordinary Go
- ordinary Ard programs do not depend on `ardgo` runtime helpers as their default implementation strategy
- interop with the Go ecosystem happens through explicit FFI boundaries
- generated Go remains readable enough for inspection, debugging, performance work, or eventual user ejection from Ard

This is intentionally closer to a “Gleam, but for Go” direction than to a design where Ard semantics are primarily implemented by a large target-specific support runtime.

### Pillar 1: runtime-free or runtime-light codegen

The backend should increasingly lower Ard constructs directly into Go constructs instead of routing them through helper APIs.

Examples of the intended direction:

- structs lower to Go structs
- functions and methods lower to ordinary Go funcs/methods
- loops and control flow lower to straight Go statements
- lists and maps lower to slices and maps with direct operations where semantics allow
- Ard-only features like `match`, `try`, `Maybe`, and `Result` should be modeled primarily as lowering/codegen problems, not helper-runtime problems, wherever practical

The broad preference is:

- more sophistication in lowering and emission
- less dependence on helper/runtime indirection

### Pillar 2: Go ecosystem interop through FFI

The Go target should make it natural for Ard code to interoperate with the broader Go ecosystem.

That means:

- explicit FFI boundaries for host functionality
- direct integration with Go stdlib and Go packages through those bindings
- target-specific extern implementations where needed

This is already compatible with Ard's broader multi-target story because extern declarations can provide target-specific bindings.

### Design constraints implied by this vision

1. **Readable generated Go matters**
   - Generated Go is not intended to be hand-edited.
   - It should still be readable enough that users can inspect it, debug it, understand performance characteristics, or choose to eject from Ard later.

2. **FFI should be explicit, not hidden**
   - Interop with Go should happen through clear extern boundaries rather than through accidental dependency on generic runtime helper layers.

3. **Compiler complexity is preferable to runtime complexity**
   - When forced to choose, the backend should generally prefer smarter lowering/codegen over growing a large helper ABI for ordinary Ard language features.

## Construct-specific design decisions

The following construct-focused notes currently serve as accepted ADRs for the Go target direction:

- `backlog/go-transpilation/match.md`
- `backlog/go-transpilation/result-maybe.md`
- `backlog/go-transpilation/unions.md`
- `backlog/go-transpilation/traits.md`

Current accepted directions:

- `match`: desugar early in backend IR, then emit ordinary Go branch forms
- `Result` / `Maybe`: use stable generated generic types internally; use FFI-specific mappings at host boundaries
- unions: use tagged generated structs with inline non-pointer payload storage (B3)
- traits: lower directly to Go interfaces; handle coercion/value-shaping in backend IR

These ADRs should be treated as the current design baseline for future Go target work unless and until they are explicitly revised.

## Candidate optimization work

The backend now has a structured pipeline, which creates room for future optimization work without reintroducing source-first emission.

Possible areas:

- dead temporary elimination
- return simplification
- redundant copy elimination where Ard semantics still permit it
- helper/import pruning beyond the current lightweight cleanup
- expression/statement normalization for simpler generated Go
- generated code shape improvements aimed specifically at faster downstream `go build`

## Richer backend IR (optional future direction)

The current implementation uses a lightweight file IR plus direct structured Go AST lowering.

A future direction would be to introduce a richer backend-local IR for statements and expressions where that meaningfully helps with:

- analysis
- optimization
- compile-time performance work
- simplification of complex lowering paths

If this happens, it should extend the structured pipeline rather than reintroduce raw-source emit/parse cycles.

## Generated project layout and packaging

Areas that may still deserve explicit design work:

- whether generated project layout should stay exactly as-is or become more configurable
- whether Go module path derivation from `ard.toml` should evolve further
- whether there should be a clearer distinction between generated user modules and generated stdlib modules
- whether the generated project tree should support additional build/test/cache modes

## Stdlib strategy

Possible future questions:

- should more stdlib support remain generated on demand versus being provided in precompiled/pre-generated forms
- should some stdlib modules gain dedicated generation shortcuts similar to special-module handling
- should stdlib generation receive its own benchmark and profiling coverage
- how much of the current stdlib should lower to plain Go operations on the Go target instead of going through generic helper/runtime shims

## Extern and helper surface

Potential future improvements:

- clearer extension points for extern registration
- better diagnostics for missing or mismatched extern implementations
- narrower helper imports where generated code only needs subsets of support functionality
- more explicit conventions for helper-backed traits and coercions
- replacing registry-based stdlib extern dispatch on the Go target with direct typed calls where the implementation is known at transpile time
- shrinking or removing parts of `compiler/go` that only exist to emulate operations the backend can now emit directly as Go syntax

## Current brainstorming priorities

These are the current high-priority improvement themes for the Go target.

### 1. Simpler lowering for `if`/`match` expressions

Status: substantially complete.

The backend now normalizes the main expression-shaped `if`/`match` paths into more statement-oriented backend IR before emission.

Implemented direction:

- prefer statement-oriented lowering where possible
- synthesize temps plus straight-line assignment/control flow instead of nested closures
- keep single-evaluation semantics for subjects/conditions explicit in backend IR
- use generated code shape closer to what a human Go programmer would write

Completed work in this area:

- normalization pass for rich expression statements
- hoisting of rich nested expressions out of deeper expression positions
- `return if ...` lowering into explicit `if` statements
- value-producing `match` lowering into synthesized temps plus assignment/return flow
- cleaner assignment rewriting for nested `if` branches and panic paths
- regression coverage for bool, enum, int/range, conditional, union, option, and result `match` shapes, including nested expression-position cases

Remaining follow-up is smaller in scope and should be treated as cleanup rather than a primary design task:

- identify any remaining exotic expression forms that still need closure-like shaping
- reduce redundant synthesized temps where output can be simplified further
- add more parity coverage as new corner cases are discovered

### 2. Reduce helper-backed codegen when plain Go is sufficient

Status: substantially complete for the straightforward list/map cases originally targeted here.

A number of operations that previously flowed through `ardgo` helpers now emit directly as ordinary Go syntax or small local helper shapes.

Completed work in this area:

- list append/prepend/set/swap patterns now emit directly
- direct map set/drop/read/has patterns now emit directly
- `map_keys` no longer depends on the runtime package helper and instead uses a generated local helper
- empty list/map literals emit directly as native Go composite literals
- dead runtime helper files for list/map mutation and map get paths were removed once backend emission no longer referenced them

Goals realized so far:

- cleaner generated Go
- less helper/runtime surface area
- better downstream Go compiler optimization opportunities
- fewer allocations and less call overhead in tight loops

Remaining work should be treated as follow-up cleanup rather than the main body of this item:

- reduce helper-backed codegen for `Maybe`/`Result` combinators where direct Go is practical
- shrink trait/coercion helper usage where plain typed Go is sufficient
- continue removing fallback helper paths when direct native emission can preserve semantics cleanly

### 3. Improve how Ard stdlib transpiles to Go

`decode_pipeline` is a strong signal that stdlib transpilation quality matters at least as much as core expression lowering. Even when user code lowers cleanly, generated stdlib modules can still dominate runtime cost.

Current focus areas:

- identify stdlib modules that are still effectively routed through generic runtime machinery on the Go target
- allow known stdlib functionality to compile down to direct typed Go calls
- avoid reflection-heavy or `any`-heavy bridging in hot paths when the target is Go and the types are known
- benchmark stdlib-generated code separately from user-code lowering so regressions are easier to localize

### 4. Remove registry-based stdlib extern dispatch on the Go target

For the Go target specifically, a runtime extern registry is often unnecessary when the implementation is known ahead of time.

Current direction:

- transpile stdlib extern calls to direct functions instead of `CallExtern("...")`
- avoid `CoerceExtern` on hot paths where direct typed return values can be emitted
- reserve dynamic/registry-style dispatch for cases that truly need it, rather than for all stdlib calls uniformly

Likely benefits:

- reduced reflection/coercion overhead
- simpler generated stdlib code
- clearer correspondence between Ard stdlib APIs and emitted Go
- a smaller `ardgo` runtime ABI

## Go-backed test mode

A substantial future feature is a dedicated Go-backed `ard test` flow.

### Backend selection

`ard test` should likely continue to avoid a `--target` flag.

Instead:
- `ard build` / `ard run` may continue to support `--target`
- `ard test` can use the backend selected in `ard.toml`
- if `target` is omitted, `ard test` can default to bytecode like the rest of the toolchain

### Goals

A Go-backed test mode should aim to:

- preserve Ard's current test semantics: `test fn`, `Void!Str`, pass/fail/panic
- preserve the visibility split between co-located tests and `/test` integration tests
- reuse Go's `go test` runner when the configured backend is Go
- provide a backend validation harness using real Ard tests

### Lowering model

An Ard test should remain Ard-shaped in semantics even when run through Go.

Conceptually:

```ard
test fn adds() Void!Str {
  try testing::assert(1 + 1 == 2, "bad math")
  testing::pass()
}
```

could lower to something like:

```go
func ardTestAdds() ardgo.Result[struct{}, string] {
    // transpiled Ard body
}

func TestAdds(t *testing.T) {
    res := ardTestAdds()
    if res.IsErr() {
        t.Fatal(res.UnwrapErr())
    }
}
```

This preserves Ard as the semantic source of truth:
- `Result::ok(())` => pass
- `Result::err(msg)` => fail
- panic => failing Go test

`testing::assert`, `testing::fail`, and `testing::pass` should remain ordinary Ard stdlib functions rather than being rewritten directly into `testing.T` calls.

### Co-located tests

A future Go test mode should likely:

- emit normal module code as usual
- emit sibling `_test.go` wrappers in the same Go package
- include transpiled `test fn` declarations only in test mode

That would preserve intended same-module/internal visibility for co-located tests.

### `/test` integration tests

A future Go test mode should likely treat `/test` Ard files as external modules with public-API visibility only.

Each such package could also receive `_test.go` wrappers that invoke the transpiled Ard test functions.

### Test-mode output shape

A future dedicated test mode should probably:

- generate a separate temporary or dedicated Go test project tree
- invoke `go test` instead of `go build` / `go run`
- keep test-only output separate from normal production `generated/` output

### Why this matters

Once implemented, Go-backed `ard test` can serve both as:

- a product feature
- a backend semantic regression harness

That would create a natural path to:

- run Ard tests through the Go backend
- validate new lowering work on real projects
- compare bytecode and Go backend behavior on the same test corpus

## Benchmarking and profiling backlog

Useful follow-up benchmarking/profiling work could include:

- stage-by-stage timing inside the Go backend pipeline
- end-to-end comparisons of current branch behavior versus `main` as the backend evolves
- larger multi-module workloads beyond current sample coverage
- separating backend time from downstream `go build` time in routine measurements
- tracking generated file size / declaration count / helper usage alongside wall-clock timings

## Open questions

1. **Go module path derivation**
   - Is the current `ard.toml`-driven module naming sufficient long term?
   - Should Go module path configuration become more explicit?

2. **Cycle handling**
   - If Ard module graphs and Go package rules diverge in edge cases, should the backend introduce extra handling or diagnostics?

3. **Optimization scope**
   - Which optimizations belong in backend lowering versus file-IR optimization versus future richer IR stages?

4. **Generated-code performance goals**
   - Should the backend optimize primarily for readability, transpile speed, downstream `go build` speed, or runtime performance of the resulting Go binary?

5. **Test-mode UX**
   - What is the cleanest CLI and generated-project story for Go-backed `ard test`?

## Non-goals for this file

This document should not try to describe the current implementation in detail.

It should not be used for:

- current package layout documentation
- current pipeline documentation
- current generated output shape documentation
- current backend behavior guarantees

Those belong in implementation docs and code-adjacent architecture docs, not in this backlog note.
