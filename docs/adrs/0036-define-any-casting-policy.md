# 0036: Define Any Casting Policy

## Status

Accepted

## Context

ADR 0031 defines `Any` as the Ard surface for Go's `any`. `Any` lowers to Go `any` and replaces the older dynamic object model. It exists primarily for FFI boundaries: Ard values can be boxed into `Any`, Go functions that accept `any` can receive Ard values, and Go functions that return `any` produce Ard `Any` values.

That leaves an important question: how does Ard code recover a concrete type from an `Any`?

Go already has a clear mechanism for this: type assertions and type switches. Ard should provide a small, checked equivalent without turning `Any` into a dynamic object system. In particular, `Any` should not gain field access, methods, pattern matching by runtime type, implicit unboxing, or unchecked casts.

Any richer interpretation of untyped external data — JSON decoding, SQL row decoding, reflection-heavy conversions, domain-specific validation, or compatibility shims — should happen through explicit libraries or Go FFI shim packages. The core language only needs a checked way to ask whether an `Any` contains a value compatible with a requested Ard type.

## Decision

Add a compiler-backed `ard/any::cast` operation with this source-level shape:

```ard
fn cast(value: Any) $T?
```

It is invoked with an explicit type argument:

```ard
use ard/any

let value: Any = "hello"
let text = any::cast<Str>(value)
```

`any::cast` is the only built-in way to recover a typed value from `Any`. It is checked and fallible: success returns `some(value)`, failure returns `none`.

### `Any` remains opaque

The following remain invalid in core Ard:

```ard
let value: Any = "hello"
let text: Str = value   // no implicit unboxing
value.name              // no field access
value.to_str()          // no methods
value == value          // no built-in equality
```

Ard does not add type-pattern matching over `Any`, unchecked casts, reflection APIs, or `as` syntax as part of this decision.

### Value casts

A cast to an immutable/value type recovers an Ard semantic value:

```ard
any::cast<Str>(value) // Str?
any::cast<User>(value) // User?
```

For a target type `T`, `any::cast<T>(value)` succeeds when the boxed Go value is either:

- the Go representation of `T`; or
- a non-nil pointer/reference representation of `T`, in which case the result is dereferenced and copied out as a `T` value.

For example, for `Str`, the generated check accepts both `string` and `*string`:

```go
switch v := value.(type) {
case string:
    // some(v)
case *string:
    if v == nil {
        // none
    }
    // some(*v)
default:
    // none
}
```

A value cast from a pointer does not preserve pointer identity. Mutating the Ard binding that receives the result mutates only that Ard value, not the original storage boxed inside `Any`.

### Mutable-reference casts

A cast may explicitly request mutable access:

```ard
any::cast<mut Str>(value) // (mut Str)?
any::cast<mut User>(value) // (mut User)?
```

For a target type `mut T`, `any::cast<mut T>(value)` succeeds only when the boxed Go value is a non-nil pointer/reference representation of `T`.

It does not accept a boxed value `T`, and it does not take the address of a boxed value. On success, the result is mutable access to the original storage inside the `Any` value. Mutating through that result mutates the original pointed-to value.

For example, for `mut Str`, the generated check accepts only `*string`:

```go
switch v := value.(type) {
case *string:
    if v == nil {
        // none
    }
    // some(v)
default:
    // none
}
```

This keeps mutation explicit in the type argument and follows Ard's normal mutability rules.

### Nil pointer policy

Nil pointers never produce `some`:

- `any::cast<T>(value)` where `value` contains a nil `*T` returns `none`.
- `any::cast<mut T>(value)` where `value` contains a nil `*T` returns `none`.

Ard does not introduce a general nil value or treat nil as `Maybe` absence outside this explicit cast operation.

### No coercive conversions

`any::cast` is a checked cast, not a decoder and not a coercion mechanism.

It does not convert between different scalar types:

```ard
let value: Any = 1
any::cast<Float64>(value) // none
```

It does not decode structural data into structs:

```ard
let value: Any = ["name": "Ada"]
any::cast<User>(value) // none, unless value actually contains a User
```

It does not implicitly convert named Go scalar types to their underlying Ard primitive. A boxed Go `type UserID string` is not a `Str` for `any::cast<Str>` unless a later ADR explicitly adds named-scalar unwrapping. Use a Go shim or cast to the foreign named type when that type is nameable.

### Foreign Go types

`any::cast` may target representable foreign Go types imported through `use go:`:

```ard
use go:time
use ard/any

let duration = any::cast<time::Duration>(value)
```

The same value and mutable-reference rules apply:

- `cast<time::Duration>` accepts the exact Go value representation and may accept a non-nil pointer by dereferencing and copying.
- `cast<mut time::Duration>` accepts only a non-nil pointer/reference representation and returns mutable access.

Unsupported foreign target types remain invalid cast targets.

### Rich decoding remains outside core

This decision intentionally does not add general decoding APIs. If a program needs richer conversion from `Any`, it should use ordinary Go FFI shims or future Ard libraries built on top of `Any`.

For example, a Go shim can expose Go's native checked assertion shape:

```go
func AsString(value any) (string, bool) {
    s, ok := value.(string)
    return s, ok
}
```

Ard then sees this through normal Go interop as:

```ard
ffi::AsString(value) // Str?
```

A more domain-specific shim can return `T!Str` when conversion needs diagnostics instead of absence.

## Consequences

- `Any` remains an opaque interop type rather than a dynamic object model.
- Ard gains one explicit, checked recovery operation for `Any`.
- Value casts are ergonomic because they accept both value and non-nil pointer representations, but they return value copies.
- Mutable-reference casts are explicit through `mut T` and preserve pointer identity/mutation.
- Nil pointers become `none` only at the cast boundary; Ard still has no general nil value.
- The compiler must support `mut T` as an explicit type argument for `any::cast`.
- The compiler must special-lower `any::cast` because the target type argument determines the generated Go assertion code.
- Libraries and Go shims remain responsible for coercive conversion, structural decoding, reflection-heavy behavior, and rich diagnostics.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/language-philosophy.md`
- `compiler/checker`
- `compiler/go`
