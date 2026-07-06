# 0041: Use Nullable Parameter Call Sugar

## Status

Accepted

## Context

Ard represents optionality with `Maybe<T>` / `T?`. Function parameters frequently use nullable values for optional inputs, but requiring callers to always write `maybe::none()` or `maybe::some(value)` adds ceremony at every call site.

The channel constructor is a motivating example:

```ard
fn new<$T>(capacity: Int?) Chan<$T>
```

Callers should be able to omit `capacity` for an unbuffered channel or pass an `Int` directly for a buffered channel without writing the `Maybe` constructors manually.

This is not a general default-argument system. It is call-site sugar for parameters whose declared type is nullable.

## Decision

For any callable parameter declared as `T?` / `Maybe<T>`, the call site may provide either `T` or `T?`.

```ard
fn f(value: Int?) {}

f(1)              // compiler wraps as some(1)
f(maybe::some(1)) // passed through
f(maybe::none())  // passed through
```

When a nullable parameter is omitted, the compiler synthesizes `none` for that parameter.

For positional calls, omitted nullable parameters must be trailing. This avoids ambiguity and preserves ordinary positional meaning:

```ard
fn f(a: Int, b: Str?, c: Bool?) {}

f(1)            // b = none, c = none
f(1, "x")       // b = some("x"), c = none
f(1, "x", true) // b = some("x"), c = some(true)
```

A positional call does not skip over a nullable parameter to bind a later required parameter:

```ard
fn f(a: Int?, b: Str) {}

f("x") // invalid; "x" is still the positional argument for a
```

Named calls can omit nullable parameters anywhere because the parameter names remove ambiguity:

```ard
fn f(a: Int?, b: Str) {}

f(b: "x") // a = none
```

The sugar applies uniformly to ordinary functions, static functions, methods, function values, module functions, and compiler-backed foreign/intrinsic functions whose Ard-facing parameter type is nullable. Direct `use go:` imports do not currently surface nullable parameter types because Go has no `Maybe<T>` parameter shape; if a future foreign target does expose an Ard-facing nullable parameter, the same call rules apply.

## Consequences

- Nullable parameters provide lightweight optional inputs without a separate default-argument feature.
- Callers can pass plain values to nullable parameters, and the checker inserts `some(value)` when the value is compatible with the inner type.
- Omitted nullable parameters lower as explicit `none` values, so backend and AIR semantics remain ordinary `Maybe` semantics.
- Positional calls remain unambiguous because only trailing nullable parameters can be omitted.
- Named calls can skip nullable parameters in any position.

## Related

- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
