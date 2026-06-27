# 0032: Select on Channels

## Status

Accepted

Supersedes the channel-API and "no `select`" guidance of `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`.

## Context

Typed channels now exist as a compiler intrinsic that lowers to a native Go `chan T` (`docs/adrs/0031-go-backend-lowering-contract.md`: "typed channels lower to native Go `chan T` rather than a runtime type"). With channels in place, Ard needs a way to wait on several channel operations at once and proceed with whichever is ready — Go's `select`. Without it, programs can only block on a single channel at a time, which makes fan-in, timeouts, and cancellation impossible to express.

Two prior decisions constrain this work, and both need revisiting:

1. ADR 0019 said: "Do not add Go-style `select` ... If Ard later adds Go-select-like multiplexing ... prefer a channel-aware `match` form instead of introducing a new `select` keyword." It also reserved that any such form must register all channel operations together, block until one is ready, run the matching arm, and that cancellation must be a separate token rather than a meaning of channel close.

2. ADR 0019's channel **API** has already been superseded in practice by the channels that shipped under 0031:
   - the module is `ard/channel`, not `ard/async/channel`;
   - construction is `channel::new<T>(capacity)`, and `send`/`recv`/`close` are **methods on `Chan<T>`** (`ch.send(x)`, `ch.recv()`, `ch.close()`);
   - there is no `Channel<T>` wrapper — `Chan<T>` is the public type;
   - `send`/`close` return `Void` with native panic-on-closed semantics, not a recoverable `Bool`;
   - operations lower directly to native `chan T` rather than through FFI.

Go's `select` cases are heterogeneous: each case operates on its own channel, potentially of a different element type, and mixes receive and send operations with a non-blocking `default`. That rules out expressing it as a homogeneous library function and confirms it needs a language construct. The design goal is to lower to Go's native `select` while reading like the rest of Ard rather than importing Go's `<-`, `:=`, and `, ok` channel syntax.

## Decision

### Ratify the shipped channel API

`ard/channel` exposes the constructor `channel::new<$T>(capacity: Int) Chan<$T>` plus the `Chan<$T>` type. The operations are methods on `Chan<$T>`:

- `Chan<$T>.send(value: $T) Void` — blocks until the value is accepted or buffered. Sending on a closed channel **panics** (native Go semantics).
- `Chan<$T>.recv() $T?` — blocks until a value is available or the channel is closed and drained. Returns `some(value)` for a received value and `none` after close-and-drain.
- `Chan<$T>.close() Void` — closes the channel for sending. Closing should be done by the sending side. Closing an already-closed channel panics.

`recv() == none` remains the closed-and-drained signal. Channels are reference-like handles; send/recv/close do not require a `mut` binding. This replaces ADR 0019's `ard/async/channel`, `Channel<$T>` wrapper, `Bool`-returning send/close, and FFI-backed operations.

### Add a `select` construct

Introduce a `select` keyword. `select` registers all of its channel operations together, blocks until one can proceed, then runs that arm's body. It is not sequential. If several arms are ready, one is chosen (Go's pseudo-random fairness). A `_` arm makes the whole `select` non-blocking.

Arms are ordinary Ard expressions, not a bespoke pattern language. An arm is:

```
( "let" IDENT "=" )? <channel-op> "=>" <body>
| "_" "=>" <body>
```

where `<channel-op>` is `<expr>.recv()` or `<expr>.send(<expr>)` and the receiver `<expr>` evaluates to a `Chan<$T>`. The channel selector may be any expression that yields a channel (`pool[i].recv()`, `getChan().recv()`); only the trailing operation is restricted to `recv()`/`send(...)`. The checker rejects any other expression as an arm head. `let` is only valid on `recv()`, since `send` returns `Void`.

```ard
let timeout = channel::new<Int>(0)   // or sourced from Go via use go: (Layer 2)

select {
  let maybe_job = jobs.recv() => match maybe_job {
    job => run(job),   // received a value
    _   => drain(),    // jobs closed (recv() == none)
  },
  results.send(value) => recorded(),   // send arm
  timeout.recv()      => giveUp(),     // receive and discard
  _                   => idle(),       // non-blocking default
}
```

Arm semantics:

- **`let name = ch.recv()`** binds `name: $T?`. A closed channel surfaces as `none`; the body handles it with an ordinary `match`, exactly like any other `$T?`. There is no separate close arm and no enforced close handling — closure is a normal `Maybe` the body may handle or ignore.
- **`ch.recv()`** (no `let`) receives and discards; it fires on a received value or on close.
- **`ch.send(x)`** sends an in-scope value `x`. It fires when the send can proceed. With a `_` arm present this is a non-blocking try-send; without `_` the send participates in blocking selection.
- **`_`** runs when no other arm is ready, making the `select` non-blocking.

### Mapping to Go and lowering

| Ard arm | Go case |
|---|---|
| `let m = ch.recv() => B` | `case v, ok := <-ch: m := some(v)/none; B` |
| `ch.recv() => B` | `case <-ch: B` |
| `ch.send(x) => B` | `case ch <- x: B` |
| `_ => B` | `default: B` |

The `let`-bound receive constructs the same `Maybe` value that the `recv()` method already produces. Go's five case forms (`<-ch`, `v := <-ch`, `v, ok := <-ch`, `ch <- x`, `default`) collapse to these four Ard arms because `recv() $T?` folds Go's value and `ok` results into a single `Maybe`.

### Closed-send semantics

A send arm whose channel is closed is *ready* in Go's `select` and will panic when chosen. Ard inherits this faithfully: `send` is native and panics on a closed channel. We keep native panic-on-closed rather than reintroducing a recoverable `Bool` send. The "sender owns close" discipline is the expected way to avoid it. This keeps channels a pure native lowering with no runtime type.

### Cancellation

Cancellation is not a special form. A cancellation/`done` channel is an ordinary channel; a consumer selects on `done.recv()` (or observes its close), consistent with ADR 0019's requirement that cancellation be a separate signal rather than a meaning of channel close.

### Directional channels and Go channel imports (Layer 2 — implemented)

These build on the `select` core and are now implemented:

- **Directional channel types** are distinct Ard types: `Receiver<T>` (receive-only, `recv` only) and `Sender<T>` (send-only, `send`/`close`), alongside the bidirectional `Chan<T>`. They do not implicitly narrow, so type-checking is explicit. `channel::receiver(ch)` and `channel::sender(ch)` derive a directional view from a bidirectional channel. They lower to Go `<-chan T` / `chan<- T`, and the factory is a Go directional conversion.
- **`use go:` channel-typed imports** map Go `chan T` / `<-chan T` / `chan<- T` in imported signatures to `Chan<T>` / `Receiver<T>` / `Sender<T>`. This lets Go-sourced channels participate in `select`: `time::After(d)` returns `<-chan time.Time`, which types as a `Receiver`, so `timeout.recv()` works in a select arm and `.send` on it is a compile error. Timeouts are obtained this way rather than via a bespoke `channel::after`.

A fresh directional channel is intentionally not constructable on its own (a receive-only channel with no sender can never receive); directional channels are always derived from a bidirectional one or sourced from Go.

## Consequences

- Ard gains multi-way channel coordination (fan-in, cancellation, non-blocking send/receive, timeouts), lowering to Go's native `select`.
- `select` arms read as ordinary Ard expressions with real `let` bindings; no `<-`, `:=`, or `, ok` syntax enters the language.
- The receive arm's `$T?` binding unifies Go's value, value-plus-`ok`, and discard receive forms, and ties closure handling to the existing `Maybe` story.
- Keeping native panic-on-closed `send` preserves channels as a pure native lowering with no runtime type, at the cost that a chosen-but-closed send panics — the same hazard Go has.
- The checker must restrict `select` arm heads to channel `recv()`/`send(...)` operations and reject other expressions, and must allow `let` only on `recv()`.
- The backend must lower `select` arms to native Go `case` clauses per the mapping table, constructing a `Maybe` for `let`-bound receives.
- Directional channels (`Receiver<T>`/`Sender<T>`) and `use go:` channel-typed imports are implemented, so `select` can consume Go-sourced channels (e.g. `time::After`) and obtain timeouts without a bespoke primitive.
- ADR 0019's channel API (`ard/async/channel`, `Channel<$T>` wrapper, `Bool` send/close, FFI-backed operations) and its "channel-aware `match` instead of a `select` keyword" guidance are superseded.

## Related

- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
- `compiler/checker/std_lib.go` (`ChannelPkg`)
- `compiler/checker/types.go` (`Chan`)
- `compiler/air/lower.go` (`lowerChanMethod`, `lowerChannelCall`)
- `compiler/go/lower.go` (channel lowering)
