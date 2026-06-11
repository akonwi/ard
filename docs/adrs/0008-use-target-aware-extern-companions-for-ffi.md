# 0008: Use Target-Aware Extern Companions for FFI

## Status

Accepted

## Context

Ard standard library and project code need a way to call target-specific host implementations. The compiler currently supports Go/native output and JavaScript targets, and each target has different host integration conventions.

Embedding target behavior directly into Ard declarations or compiler special cases would make externs harder to reason about and harder to extend across targets. The FFI system should stay narrow, explicit, and focused on standard library plus project-local companion modules.

## Decision

Use target-aware extern bindings and companion modules/files for Ard FFI.

Extern function declarations may use a Go-oriented string shorthand:

```ard
extern fn read_line() Str!Str = "ReadLine"
```

Extern type declarations may also bind directly to a Go type when project or stdlib FFI should use a concrete host type instead of the default opaque representation:

```ard
extern type Terminal = "*vaxis.Vaxis"
```

or an explicit target binding block:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js = "readLine"
  js-browser = "readLineBrowser"
}
```

Supported binding keys are:

- `go`
- `js`
- `js-server`
- `js-browser`

Extern binding resolution should prefer:

1. exact target binding
2. shared `js` binding for `js-server` and `js-browser`
3. `go` fallback

Go project companions may provide project-local extern implementations in either:

- root `ffi.go`
- `ffi/*.go`

Companion files must use `package ffi`. The Go target copies project companion files into the generated workspace as a generated project FFI package named after the Ard project (sanitized for Go identifiers) and calls bindings directly. A project should not use both root `ffi.go` and `ffi/*.go` at once, so the companion layout remains unambiguous.

Project Go companion adaptation should use idiomatic generated Go values rather than a universal dynamic object layer. At the project FFI boundary, the current direct-call convention is:

- `extern type Name = "project_name.GoType"` values pass as the bound Go type; project FFI type bindings must be qualified with the generated project package name
- scalar/list/map/function arguments pass as generated Go values
- `T?` arguments pass as `runtime.Maybe[T]`, preserving Ard's explicit nullable representation at the host boundary
- `T` returns directly as `T`
- `T?` returns as `runtime.Maybe[T]`
- `Void!Str` maps to `error`
- `T!Str` maps to `(T, error)`

This convention keeps nullable values explicit across FFI. Go pointers in companion signatures represent reference semantics (for example `mut T`) or explicit pointer-backed `extern type` bindings, not ordinary Ard `T?` values. If an underlying Go library uses `nil` pointers for optional values, companion code should adapt that library convention into or out of `runtime.Maybe[T]` itself. Standard-library Go FFI uses generated metadata and stdlib FFI wrapper types where needed, following the same semantic distinction.

JavaScript externs should use target-specific `.mjs` companion modules. Generated JavaScript imports the companion module and calls the exported binding rather than embedding per-function module paths in Ard source.

Standard library Go FFI metadata should be generated from Ard standard library declarations and Go implementations so the Go target can route standard library extern calls consistently.

## Consequences

- Ard declarations stay explicit about target bindings while target-specific behavior lives in companion code.
- The FFI surface remains narrow and easier to validate.
- Go project FFI can use direct idiomatic calls without a universal boxed runtime representation.
- JavaScript targets can use ESM companion modules that match their runtime environment.
- Unsupported companion layouts or signatures should be rejected early.
- Future userland FFI code generation can build on this companion model without changing the core extern binding decision.

## Related

- `docs/adrs/0016-defer-project-ffi-codegen.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
