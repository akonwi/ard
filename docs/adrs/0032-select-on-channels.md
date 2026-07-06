# 0032: Select on Channels

## Status

Accepted

Builds on [ADR 0019](0019-use-typed-channels-for-fiber-communication.md), which defines `Chan<T>`, `Receiver<T>`, `Sender<T>`, `Chan::new`, and `ard/async::start`.

## Context

Typed channels let Ard programs communicate between concurrently running tasks. Programs also need a way to wait on several channel operations at once and proceed with whichever operation is ready. Go provides this with `select`, and Ard should expose the same coordination power while keeping Ard syntax readable and avoiding Go's `<-`, `:=`, and comma-ok receive forms.

A select operation cannot be a normal homogeneous function: each arm may use a different channel element type, and arms may mix sends, receives, and a non-blocking default. It needs to be a language construct that the checker and backend understand.

## Decision

Add a `select` construct for channel multiplexing.

`select` registers all of its channel operations together, blocks until one can proceed, then runs that arm's body. If several arms are ready, one is chosen according to the target's native fairness behavior. On Go this is Go's pseudo-random select choice. A `_` arm makes the whole `select` non-blocking.

### Arm syntax

An arm is one of:

```text
let IDENT = <receive-op> => <body>
<receive-op> => <body>
<send-op> => <body>
_ => <body>
```

where:

- `<receive-op>` is an expression ending in `.recv()` whose receiver has type `Chan<T>` or `Receiver<T>`;
- `<send-op>` is an expression ending in `.send(value)` whose receiver has type `Chan<T>` or `Sender<T>`;
- `let` binding is valid only for receive arms;
- `_` is the default arm.

Examples:

```ard
use go:fmt
use go:time

let jobs = Chan::new<Str>()
let done = Chan::new<Void>()
let timeout = time::After(duration)

select {
  let maybe_job = jobs.recv() => match maybe_job {
    job => fmt::Println(job)
    _ => fmt::Println("jobs closed")
  }
  done.recv() => fmt::Println("done")
  jobs.send("next") => fmt::Println("sent")
  time::After(duration).recv() => fmt::Println("timeout")
  _ => fmt::Println("idle")
}
```

The channel expression does not need to be a simple name. It may be any expression that yields a channel, including a direct Go call such as `time::After(duration)`. That enables the Go idiom of declaring timeout channels directly in select arms:

```ard
select {
  time::After(duration).recv() => timeout()
  work.recv() => handle()
}
```

### Receive arms

`let name = ch.recv()` binds `name: T?`.

- `some(value)` means a value was received.
- `none` means the channel is closed and drained.

`ch.recv()` without `let` receives and discards. It fires both for a received value and for closed-and-drained completion.

### Send arms

`ch.send(value)` participates in the select. It fires when the send can proceed. With no `_` arm, the select blocks until a send or receive arm is ready. With `_`, the send behaves like a non-blocking try-send arm.

Sending on a closed channel preserves native channel semantics and panics when selected.

### Default arm

`_ => body` is the default arm. It runs only when no send or receive arm can proceed immediately, making the select non-blocking.

### Go lowering

On the Go target, Ard select lowers to native Go `select`:

| Ard arm | Go case |
| --- | --- |
| `let m = ch.recv() => B` | `case v, ok := <-ch: m := some(v)/none; B` |
| `ch.recv() => B` | `case <-ch: B` |
| `ch.send(x) => B` | `case ch <- x: B` |
| `_ => B` | `default: B` |

The let-bound receive constructs the same `Maybe<T>` shape used by ordinary `recv()`.

### Directional channels and Go interop

Directional channel types from ADR 0019 participate in select:

- `Receiver<T>` supports receive arms only.
- `Sender<T>` supports send arms only.
- `Chan<T>` supports both.

Direct Go imports map Go channel types into Ard channel types:

| Go | Ard |
| --- | --- |
| `chan T` | `Chan<T>` |
| `<-chan T` | `Receiver<T>` |
| `chan<- T` | `Sender<T>` |

This lets Go-sourced channels participate in Ard select. For example, Go's `time.After` returns `<-chan time.Time`, so Ard sees a `Receiver<time::Time>` and can write `time::After(duration).recv()` directly in a select arm.

### Cancellation

Cancellation is not a special construct. A cancellation or done signal is an ordinary channel. Code selects on `done.recv()` or observes channel close through `none`.

## Consequences

- Ard gains multi-way channel coordination for fan-in, cancellation, non-blocking send/receive, and timeouts.
- Select arms use Ard method syntax and `Maybe`, not Go receive syntax.
- The checker must restrict select arm heads to channel `recv()`/`send(...)` operations and must reject `let` on send arms.
- The backend must lower select to native target select machinery where available. The Go target lowers directly to Go `select`.
- Direct Go channel-returning calls can be used inline as select arm receivers.
- Closed-send panic behavior is preserved from native channels.

## Related

- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
