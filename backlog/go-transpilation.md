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

## Extern and helper surface

Potential future improvements:

- clearer extension points for extern registration
- better diagnostics for missing or mismatched extern implementations
- narrower helper imports where generated code only needs subsets of support functionality
- more explicit conventions for helper-backed traits and coercions

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
