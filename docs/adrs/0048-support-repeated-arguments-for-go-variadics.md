# 0048: Support Repeated Arguments for Go Variadic Calls

## Status

Accepted

## Context

Direct Go FFI already recognizes Go variadic parameters. A trailing Go variadic parameter is currently exposed as one optional Ard argument, which makes zero-argument and one-argument calls possible:

```ard
fmt::Println()
fmt::Println("hello")
```

However, ordinary Go variadic calls commonly pass multiple trailing values:

```go
fmt.Println("hello", "world", 42)
```

Today the equivalent Ard call is rejected with an arity error because Ard sees only one trailing variadic slot. Issue #246 also discussed Ard slice/list spread syntax, but spread has broader language-design implications and can be deferred.

## Decision

Ard will support calling foreign Go functions and methods that have variadic parameters by allowing repeated trailing arguments at the call site.

Ard itself will not gain variadic function declarations in this change. User-defined Ard functions still have fixed arity.

For a Go function with parameters equivalent to:

```text
(p1: A, p2: B, rest: ...T)
```

Ard accepts calls with at least the non-variadic argument count:

```ard
f(a, b)
f(a, b, x)
f(a, b, x, y)
```

The checker treats every argument in the variadic segment as a separate value of the variadic element type `T`. Existing Go FFI argument adaptations, such as interface upcasts and scalar conversions at the boundary, apply to each repeated variadic argument independently.

Lowering emits a normal Go variadic call with repeated arguments:

```go
f(a, b, x, y)
```

No Ard spread syntax is introduced in this ADR.

## Non-goals

This change does not add:

- Ard variadic function declarations.
- Ard spread syntax such as `args...`.
- Passing an Ard list as a variadic spread.
- Mixing fixed repeated variadic values with a final spread.
- General variadic behavior for non-Go functions.

Spread syntax can be considered later as a separate design if forwarding list/slice values into Go variadics becomes important.

## Consequences

This preserves Ard's fixed-arity function model while making common Direct Go FFI calls ergonomic. It also keeps hidden allocation/copy behavior out of the initial design: repeated arguments lower directly to Go repeated arguments, and list/slice spreading remains explicit future work.
