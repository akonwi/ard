# 0003: Use Generic Fibers for Async Eval

## Status

Superseded by `docs/adrs/0033-async-is-goroutines-and-channels.md`.

The `Fiber<$T>` / `runtime.Fiber` model, `async::eval`/`join`/`get`, and the
fiber capture-isolation rule described below are no longer in effect. Async is
now just `async::start` (a fire-and-forget goroutine) plus channels, and
result-returning/joining ergonomics are ordinary userland Ard. The historical
decision is preserved below for context.

## Context

Ard has an `ard/async` module for concurrent work. `async::start(fn() Void)` supports fire-and-wait style concurrency, but programs also need to run concurrent computations that produce typed results.

A result-producing async API needs to preserve Ard's type-safety goals while keeping synchronization explicit. It also needs to share the same capture isolation rules as other async work so concurrent closures cannot mutate captured outer state unsafely.

## Decision

Represent result-producing async work with generic `Fiber<$T>` values.

The async API should distinguish synchronization from result retrieval:

- `async::start(fn() Void) Fiber<Void>` starts `Void` work.
- `async::eval(fn() $T) Fiber<$T>` starts work that produces `$T`.
- `Fiber<$T>.join()` waits for completion and discards the result.
- `Fiber<$T>.get() $T` waits for completion and returns the result.
- `async::join([Fiber<$T>])` waits for a list of fibers when only synchronization is needed.

A `Fiber` carries the concurrency handle plus a generic result field:

```ard
struct Fiber {
  wg: WaitGroup,
  result: $T,
}
```

`Fiber.get()` waits before retrieving the stored result, so callers do not need to call `join()` separately when they need the value.

`async::start` and `async::eval` should enforce the same fiber isolation rules: closures may capture read-only outer values, but must not capture mutable outer variables.

## Consequences

- Async result retrieval remains statically typed through `Fiber<$T>`.
- Waiting and value retrieval are separate operations, making intent clear at call sites.
- `async::start` remains a convenience for `Void`-returning work.
- The checker must preserve and validate fiber generic types and capture isolation rules.
- Targets that support Ard async must store and retrieve fiber results in a way that preserves the declared `$T` result type.
- Targets that do not support Ard's fiber model need explicit compatibility decisions rather than silently approximating it.

## Related

- `compiler/std_lib/async.ard`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
