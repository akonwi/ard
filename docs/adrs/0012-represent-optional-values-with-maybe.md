# 0012: Represent Optional Values with Maybe

## Status

Accepted

## Context

Ard needs a type-safe way to represent values that may be absent. Using sentinel values such as empty strings, zero values, or `null`-like dynamic values would make absence implicit and weaken static checking.

Optional values also need to work with generic helpers and with `try`-based early return so absence can be propagated explicitly.

## Decision

Represent optional values with `Maybe<T>`, written using the shorthand `T?`.

A `Maybe<T>` value is either:

- `some(T)`, representing a present value
- `none`, representing absence

The standard library should provide constructors such as `maybe::some(value)` and `maybe::none()`. `maybe::none()` is generic and may be resolved from context when an expected `Maybe<T>` type is available.

Maybe values should support ergonomic operations such as `.or(default)` to provide a fallback value of the inner type.

Type equality for Maybe values is based on the inner type: `Maybe<T>` is compatible with `Maybe<U>` only when `T` and `U` are compatible.

`Maybe` participates in Ard's recoverable control-flow model: `try` can unwrap a present value or return early on `none` when the current function returns a compatible Maybe type, and catch blocks can transform absence into another return value.

## Consequences

- Absence is explicit in function signatures and checked statically.
- APIs can distinguish missing values from valid zero/empty values.
- `maybe::none()` requires contextual generic inference in some expressions.
- Backends need concrete representations for present/absent state while preserving the typed inner value.
- FFI boundaries need clear adaptation rules for optional values.
- Maybe semantics compose with the generic type system and `try`-based error handling.

## Related

- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0010-support-dollar-prefixed-generics.md`
