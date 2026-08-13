# 0058: Preserve Go Variadic Callable Types

## Status

Accepted

## Context

ADR 0048 made direct Go variadic function and method calls accept zero or repeated trailing arguments:

```ard
fmt::Println()
fmt::Println("hello", 42)
```

First-class references did not preserve that call shape. A captured Go variadic function was rewritten to a fixed Ard function with a trailing `Maybe<T>` parameter. That representation could express only zero or one trailing value, diverged from direct calls, and leaked a stale `Variadic` flag that allowed calls the generated fixed adapter could not accept. Bound method values had a separate lowering path and exhibited the same inconsistency.

Variadicity is a property of a callable type. Keeping it only on a foreign expression cannot survive assignment, parameters, fields, or returns.

## Decision

A Go variadic callable retains one canonical trailing element parameter marked variadic. For a Go signature equivalent to:

```go
func(A, ...T) R
```

its Ard callable type is:

```ard
fn(A, ...T) R
```

The marker is part of function-type identity and is preserved by the checker and AIR. Only the final parameter may be variadic. Its type is the repeated element type `T`, not `Maybe<T>`.

Calls through direct package access, direct method access, package function values, bound method values, and named Go function values use the same arity rules:

- the minimum argument count excludes the variadic parameter;
- every repeated argument is checked independently against the element type;
- zero repeated arguments pass nothing;
- ordinary calls lower with repeated arguments, without a spread operation.

Function-type syntax accepts `...T` only in the final position. This describes foreign callable values crossing explicitly typed assignments, parameters, fields, and returns. It does not permit variadic Ard declarations or closures.

Variadicity is represented on AIR function types and therefore participates in type interning, structural equivalence, validation, and Go function-type rendering. Fixed and variadic function types are distinct.

Go lowering uses native variadic function values whenever their parameter ABI is already exact. When another foreign boundary rule requires a wrapper, such as projecting an Ard list reference to a Go slice descriptor, the wrapper is itself variadic and forwards its generated Go slice internally:

```go
func(fixed A, tail ...T) R {
    return foreign(fixed, tail...)
}
```

Package functions and bound methods share this foreign-callable adaptation rule. A bound receiver is evaluated once.

## Non-goals

This decision does not add:

- Ard variadic function declarations;
- variadic Ard closures;
- call-site spread or list forwarding;
- implicit conversion between fixed and variadic function types;
- first-class references to unspecialized generic Go functions.

## Consequences

Captured Go variadic functions and methods now behave like direct calls and can accept any number of repeated trailing values. Variadic metadata survives ordinary first-class value flow.

The former trailing-`Maybe<T>` convention is removed. `Maybe::new()` is no longer a sentinel for omitting a variadic argument; it is either an actual argument value or a type error.

Generated Go uses native variadic function types and introduces no hidden tail allocation for ordinary captured calls. Internal `tail...` forwarding appears only in generated wrappers required by another ABI adaptation.

## Related

- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0048-support-repeated-arguments-for-go-variadics.md`
- Issue #389
