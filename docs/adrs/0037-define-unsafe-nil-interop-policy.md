# 0037: Define Unsafe Nil Interop Policy

## Status

Accepted

## Context

Direct Go interop exposes Go value states that are not Ard semantics. The most important one is `nil`: Go pointers, slices, maps, channels, functions, interfaces, and typed nil values can be nil even when their Ard-facing type is not `Maybe`.

Ard already has explicit absence through `Maybe`. Treating every Go nil as `none` would blur that semantic distinction and would misrepresent many Go APIs where nil means default behavior, uninitialized storage, a sentinel value, or a panic-prone invalid state rather than domain absence.

At the same time, Ard code needs a way to inspect foreign nil state when building safe wrappers over direct Go APIs.

## Decision

Go nil is a foreign/backend value state, not Ard absence.

- Direct Go pointer/reference-like values do not implicitly become `Maybe`.
- Go nil does not implicitly lower to `none`.
- Ard does not add a general `nil` literal.
- Ard wrappers that want absence semantics must explicitly adapt foreign nil into `Maybe`.

Add a compiler-backed standard-library intrinsic:

```ard
use ard/unsafe

unsafe::is_nil(value: Any) Bool
```

`unsafe::is_nil` is part of `ard/unsafe` because it inspects backend/runtime value state outside Ard's safe core semantics. It accepts `Any`, so any representable Ard value can be passed through ordinary `Any` boxing.

For the Go backend, `unsafe::is_nil` returns `true` for nil-able Go values whose runtime value is nil, including typed nil values boxed in `any`:

- nil interface values;
- nil pointers;
- nil slices;
- nil maps;
- nil channels;
- nil functions;
- nil interfaces.

It returns `false` for non-nil-able values such as strings, numbers, booleans, and structs, and for non-nil nil-able values.

The Go backend lowers this intrinsic to a runtime helper that uses reflection after first checking `value == nil`. Including this helper in the runtime is acceptable because it supports a compiler-backed standard-library intrinsic; it is not a new runtime value representation.

## Consequences

- Ard keeps one nullability model: `Maybe`.
- Direct Go interop preserves Go nil behavior instead of silently adapting it.
- Safe Ard APIs can explicitly translate foreign nil into `Maybe` where that is the desired domain model.
- `ard/unsafe` now contains compiler-backed operations for inspecting opaque or foreign runtime state: `unsafe::cast` and `unsafe::is_nil`.
- Future backends must define `unsafe::is_nil` deliberately. A backend with no nil-like foreign value state may lower it to `false`.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/adrs/0036-define-any-casting-policy.md`
