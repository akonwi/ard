# 0024: Preserve Maybe Semantics in Go Lowering

## Status

Accepted

## Context

Ard is increasingly comfortable being a source language that transpiles to idiomatic Go for native execution. That does not mean every Ard semantic concept should collapse into the closest Go idiom.

Nullable values are one place where the distinction matters. Ard represents absence with `Maybe<T>`, written as `T?`. A maybe value means exactly one of:

- `some(T)`, a present value;
- `none`, absence.

A Go pointer can also be `nil`, but pointer-ness carries additional meanings: addressability, identity, indirection, mutation, sharing, and host interop. Lowering an Ard nullable directly as `*T` makes generated Go conflate two separate Ard concepts:

- `T?` / `Maybe<T>`: presence or absence;
- `mut T`, extern host pointers, and backend implementation details: reference or address semantics.

ADR 0012 already establishes Maybe as the source-level optional representation. ADR 0020 introduced special Go lowering for recursive nullable fields, allowing fields such as `parent: Node?` to lower as pointer-backed optional fields so recursive layouts remain finite. That solved a backend layout problem, but it leaks pointer semantics into generated Go at the exact place where Ard wants an explicit Maybe concept.

The checker is responsible for recursive-layout validity. It decides which type graphs are finite and which inline cycles must be rejected. The Go backend should preserve Ard's accepted semantics instead of choosing pointer syntax as the user-visible representation for nullable values.

## Decision

The Go backend should represent Ard nullable values as a runtime-level Maybe type in generated Go.

At Ard semantic positions, every `T?` should lower as a Maybe value, not a bare pointer:

```ard
let name: Str? = maybe::none()
```

should lower conceptually as:

```go
var name runtime.Maybe[string] = runtime.None[string]()
```

not as:

```go
var name *string
```

This applies to locals, parameters, return values, struct fields, list and map elements, union/result payloads, closure captures, and direct recursive nullable fields accepted by the checker.

The runtime representation may use indirection internally to remain finite for recursive shapes. For example, `runtime.Maybe[T]` may store a private `*T` or equivalent cell internally. That implementation detail must stay behind the Maybe API; generated Ard code should still mention `runtime.Maybe[T]`, not `*T`, for nullable values.

The backend should use runtime helpers or methods for Maybe construction, presence checks, unwrapping, mapping, and defaulting instead of relying on public struct fields such as `Some` and `Value`. This keeps the runtime free to choose a finite internal representation without changing generated Go's visible semantic type.

Project and dependency Go FFI should also use the runtime Maybe representation for nullable Ard values. A hand-written companion that accepts or returns `T?` should use `runtime.Maybe[T]` or the generated stdlib FFI alias for the same type, making nullability explicit in host code too:

```go
func Lookup() runtime.Maybe[string]
func Select(input runtime.Maybe[string]) string
```

A Go pointer in a host signature should represent reference semantics, not nullable semantics. In Ard terms, a host `*T` corresponds to a mutable/reference-like value such as `mut T` or to an `extern type` whose Go binding is itself a pointer. It should not be the default representation for `T?`.

If an underlying Go library uses `nil` pointers to model optional values, the companion should adapt that library convention explicitly at the edge by constructing or inspecting `runtime.Maybe[T]`. The Ard FFI boundary should still expose Maybe for nullable values.

Extern type bindings such as `extern type Terminal = "*vaxis.Vaxis"` remain real host pointer/reference types. They are distinct from Maybe. If such a value is nullable in Ard, the signature should say so explicitly as `Terminal?`, which lowers as `runtime.Maybe[*vaxis.Vaxis]` rather than a bare `*vaxis.Vaxis`.

This decision supersedes the Go-specific pointer-backed nullable-field representation described in ADR 0020. Recursive nullable fields should still be accepted or rejected by checker-sizedness rules, but accepted nullable fields should appear in generated Go as `runtime.Maybe[T]` with a finite runtime implementation.

## Implementation Plan

1. Change `compiler/runtime/maybe.go` so `Maybe<T>` has a stable API and a finite internal representation.
   - Keep `runtime.Some(value)` and `runtime.None[T]()`.
   - Add helpers or methods for presence checks, value extraction, and optional host adaptation, such as `IsSome`, `IsNone`, and `Value`.
   - Use internal indirection if needed so `runtime.Maybe[Node]` can appear inside `Node` without requiring the field to become `*Node`.

2. Remove Go backend recursive-nullable pointer lowering.
   - Stop emitting `*T` for `FieldInfo.RecursiveNullable` struct fields.
   - Remove conversions that lower struct literal fields from `Maybe<T>` to `*T` and field reads from `*T` back to `Maybe<T>`.
   - Remove or retire `RecursiveNullable` as a Go codegen representation concern once no backend path needs it.

3. Update generated Go Maybe operations to use the runtime Maybe API.
   - `maybe::some(value)` should lower through `runtime.Some(value)`.
   - `maybe::none()` should lower through `runtime.None[T]()` when the type is known.
   - `.is_some()`, `.is_none()`, `.expect()`, `.or()`, `match`, `try`, `map`, and `and_then` should use runtime API calls instead of assuming public `.Some` and `.Value` fields.
   - Avoid changing unrelated `Result` lowering, which may still use `Ok`, `Value`, and `Err` fields.

4. Change project and dependency Go FFI nullable conventions.
   - `T?` parameters and returns should use `runtime.Maybe[T]` in companion signatures.
   - Remove project/dependency FFI pointer adaptation for ordinary `T?` values.
   - Keep Go pointers for `mut T`, explicit pointer-backed `extern type` bindings, and host-library internals.
   - Update ADR 0008 and ADR 0016 to replace the old `T?` as `*T` convention with the Maybe convention.

5. Update stdlib FFI and generated embedded files.
   - Update `compiler/std_lib/ffi` host code to use the runtime Maybe API.
   - Regenerate stdlib FFI metadata with `go generate ./std_lib/ffi`.
   - Regenerate embedded Go module files with `go generate ./go` if runtime or stdlib FFI embedded content changes.

6. Update generated JSON helpers and other backend helper code.
   - JSON encode/decode helpers should construct and inspect Maybe values through the runtime API.
   - Any helper that currently reads or writes `.Some` or `.Value` for Maybe should migrate to the stable API.

7. Add regression coverage.
   - Assert generated Go emits `runtime.Maybe[Node]` for recursive nullable struct fields, not `*Node`.
   - Keep existing recursive nullable parity tests passing.
   - Add runtime tests that `Maybe` can be used in a recursive Go type.
   - Add runtime tests that `some(nilPointer)` is distinct from `none` for pointer element types.
   - Add project FFI tests proving `T?` companion parameters and returns use `runtime.Maybe[T]`.
   - Add or update tests that generated code no longer depends on public `.Some` / `.Value` fields for Maybe.

8. Validate broadly.
   - Run `cd compiler && go test ./runtime ./air ./go -count=1` during development.
   - Run `cd compiler && go generate ./std_lib/ffi && go generate ./go && go test ./...` before review.

## Consequences

- Generated Go preserves Ard's distinction between nullable values and pointers/references.
- `mut T` and extern host pointers can own reference semantics without being confused with `T?`.
- Recursive nullable fields can remain finite without exposing raw pointers in generated Ard code, provided the runtime Maybe representation uses internal indirection.
- The Go runtime Maybe API becomes part of the backend contract. Generated code should prefer constructors/accessors over direct field reads and writes.
- Existing generated code paths that inspect `.Some` or `.Value` must migrate to helper/accessor calls or to whatever stable API the runtime Maybe type exposes.
- Project and dependency FFI signatures become more explicit: nullable Ard values use Maybe in host code, while Go pointers represent reference/extern-pointer semantics.
- Existing project FFI companions that relied on `T?` lowering as `*T` must migrate to `runtime.Maybe[T]` and perform any library-specific nil-pointer adaptation themselves.
- There may be allocation and performance tradeoffs if `some(value)` stores the value behind a pointer or cell. Optimize representation later without changing the visible Maybe type.
- Tests should assert generated Go uses `runtime.Maybe[...]` for nullable fields, values, and FFI signatures, including recursive nullable fields, and does not lower ordinary Ard nullables to bare pointers.

## Related

- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0020-support-recursive-struct-fields-through-indirection.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `compiler/runtime/maybe.go`
- `compiler/go`
