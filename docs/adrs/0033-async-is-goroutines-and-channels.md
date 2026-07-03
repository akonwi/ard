# 0033: Async is goroutines and channels

## Status

Accepted

Supersedes `docs/adrs/0003-use-generic-fibers-for-async-eval.md`.

## Context

Ard's async story began as an opinionated result-returning fiber model
(`0003-use-generic-fibers-for-async-eval.md`): a generic `Fiber<$T>` backed by a
`runtime.Fiber[T]` shape, with `async::eval`, `Fiber.join()`, `Fiber.get()`, and
`async::join`, plus a capture-isolation rule forbidding spawned closures from
capturing mutable outer state.

Two things have since changed the landscape:

- Ard can start goroutines. `async::start(fn() Void)` lowers to a goroutine.
- Ard has typed channels (`0019-use-typed-channels-for-fiber-communication.md`,
  `0031-go-backend-lowering-contract.md`, `0032-select-on-channels.md`):
  `Chan<T>`, `Receiver<T>`, and `Sender<T>` lower to native Go channel types
  with `send`/`recv`/`close` and `select`.

With both in place, every ergonomic the fiber model provided — waiting for
completion, retrieving a typed result, joining a set of tasks, wait-groups,
structured concurrency — is expressible as ordinary Ard over a channel. The one
thing userland genuinely cannot express is *starting* a goroutine, because Ard
has no `go` statement.

Keeping `runtime.Fiber`, `async::eval`, and the join/get API means baking one
opinionated coordination shape into the most privileged layer of the system —
the runtime — when it is now just library code.

## Decision

Async is `async::start` plus channels. Everything else is ordinary Ard.

- `ard/async::start(do: fn() Void) Void` is a module-shaped compiler intrinsic.
  On the Go target it runs `do` on a new goroutine. It is fire-and-forget and
  returns no handle or value.
- Coordination is userland over channels. Result-returning tasks (the old
  `eval`/`get`), joining (the old `join`), wait-groups, sleeps/timeouts, and
  structured concurrency are written in Ard against channels or direct Go
  imports such as `time::After`, not provided by the runtime.
- The runtime no longer defines a `Fiber` type. The sanctioned shared runtime
  types are `runtime.Maybe[T]` and `runtime.Result[T, E]` (see 0031).
- Spawned closures follow Go's concurrency semantics. The previous rule
  forbidding capture of mutable outer state is dropped; programs coordinate
  shared state with channels, and data races are the program's responsibility,
  exactly as in Go.

This supersedes the `Fiber<$T>` / `async::eval` / `join` / `get` model of
`0003-use-generic-fibers-for-async-eval.md` and removes `runtime.Fiber`.

## Consequences

- The runtime package is reduced to `Maybe` and `Result`; `runtime.Fiber` and
  its `SpawnFiber`/`JoinFiber`/`GetFiber` are removed.
- The `ard/async` surface is the compiler-backed `start` function; the `Fiber`
  type, `eval`, `join`, and `get` no longer exist in the standard library.
- Opinionated concurrency ergonomics move to userland and can evolve as ordinary
  libraries rather than as compiler/runtime commitments. A result-returning task
  is a struct over a `Receiver<$T>`; joining is reading channels.
- Async adopts Go-like semantics: no enforced capture isolation, so the checker
  no longer validates fiber capture rules.
- `0003`'s typed `Fiber<$T>` result-retrieval guarantee is given up at the
  language level; a userland abstraction that wants it carries its own typed
  channel.

## Related

- `docs/adrs/0003-use-generic-fibers-for-async-eval.md` (superseded)
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
- `docs/adrs/0032-select-on-channels.md`
- `compiler/checker/std_lib.go` (`AsyncPkg`)
