# 0016: Defer Project FFI Codegen

## Status

Accepted

## Context

Ard already supports project-local Go FFI companions. A project can declare extern functions and extern types in Ard, then provide hand-written Go implementations in either root `ffi.go` or `ffi/*.go` using `package ffi`. The Go target copies those companion files into the generated workspace and calls the requested binding directly.

ADR 0008 defines this companion model and the current project FFI boundary conventions:

- typed extern types can bind to concrete Go host types, such as `extern type Terminal = "*vaxis.Vaxis"`
- scalar, list, map, function, and typed extern values pass as generated Go values
- `T?` parameters cross the project FFI boundary as `runtime.Maybe[T]`
- `T?` returns cross back as `runtime.Maybe[T]`
- `Void!Str` maps to `error`
- `T!Str` maps to `(T, error)`

GitHub issue #142 proposed generating a stable project FFI contract file from Ard `extern` declarations. The intended benefit was scaffolding the Go host side so users do not need to infer the exact signatures manually.

That benefit is not compelling enough right now. The project FFI surface is usable with hand-written companions, and generating host scaffolding raises unresolved developer-experience questions. In particular, scaffolding is useful for creating an initial host file, but it is awkward as an update mechanism: once users edit host implementations, regenerated scaffolding cannot safely update those files without risking overwrites, stale stubs, or confusing split ownership between generated and user-authored code.

## Decision

Defer project FFI codegen.

Do not add `ard ffi init`, `ard ffi`, `ard ffi check`, or generated `PROJECT_ROOT/ffi/ard.gen.go` as part of the current FFI roadmap.

Continue supporting hand-written project Go FFI companions through the existing model:

```text
project/
  ard.toml
  go.mod
  main.ard
  ffi.go        # package ffi, or
  ffi/*.go      # package ffi
```

Typed extern type bindings remain the preferred way to make project FFI signatures precise when a host type should cross the boundary:

```ard
extern type Terminal = "*vaxis.Vaxis"
```

The compiler should continue to validate and document the existing companion conventions rather than introduce a generated host scaffolding workflow before its ergonomics are clear.

## Consequences

- Project FFI remains simpler: users write and own their Go companion files directly.
- The compiler avoids introducing generated files whose relationship to user-authored host implementations is ambiguous.
- There is no new stale-generated-code workflow or CI check to maintain.
- Users still need to understand the project FFI boundary conventions from ADR 0008 when writing host functions.
- Future work may revisit codegen if there is a clearer need, such as generating type-only contracts, editor hints, or diagnostics that do not create awkward host-file update semantics.
- GitHub issue #142 is closed as deferred rather than pending implementation.

## Related

- GitHub issue #142: Add userland FFI codegen
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
