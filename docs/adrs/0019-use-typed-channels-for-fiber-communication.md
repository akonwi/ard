# 0019: Use Typed Channels and Async Start for Concurrency

## Status

Accepted

Select-specific coordination is defined by [ADR 0032](0032-select-on-channels.md). Go backend lowering details are part of [ADR 0031](0031-go-backend-lowering-contract.md).

## Context

Ard needs a small concurrency model that works naturally on the Go target without making Go the definition of the language. Go's concurrency model is built around goroutines and channels, and Ard should be able to reuse that model directly when targeting Go.

At the same time, Ard should avoid importing Go-only syntax such as `<-` or a `go` keyword. Concurrency should remain explicit in Ard source and should leave room for future targets with different scheduling models.

The old FFI-backed channel design used wrapper types, `extern` functions, and recoverable `Bool` results for send/close. The FFI reset removed that machinery. Channels are core enough to be compiler-known types that lower directly to native target channel representations where supported.

## Decision

### Built-in channel types

Add typed channel types as built-in/prelude types:

```ard
Chan<$T>
Receiver<$T>
Sender<$T>
```

They do not require an import to name. On the Go target they lower to:

| Ard | Go |
| --- | --- |
| `Chan<T>` | `chan T` |
| `Receiver<T>` | `<-chan T` |
| `Sender<T>` | `chan<- T` |

`Chan<T>` is bidirectional. `Receiver<T>` can only receive. `Sender<T>` can send and close. Directional channels are not independently constructable; they are derived from a bidirectional channel or sourced from Go interop.

Channels are reference-like handles. Sending, receiving, closing, and deriving directional views do not require a `mut` binding.

### `ard/channel` API

Construction and helper operations live in `ard/channel`:

```ard
use ard/channel

let ch = channel::new<Str>()
let buffered = channel::new<Str>(10)
let rx = ch.receiver()
let tx = ch.sender()
```

The constructor has one nullable capacity parameter:

```ard
fn new<$T>(capacity: Int?) Chan<$T>
```

Omitting the nullable capacity, or passing `none`, creates an unbuffered channel. Passing `some(n)` creates a buffered channel of capacity `n`. On the Go target this lowers to `make(chan T)` or `make(chan T, n)`.

Channel operations are methods:

```ard
Chan<$T>.send(value: $T) Void
Chan<$T>.recv() $T?
Chan<$T>.close() Void
Chan<$T>.receiver() Receiver<$T>
Chan<$T>.sender() Sender<$T>

Receiver<$T>.recv() $T?

Sender<$T>.send(value: $T) Void
Sender<$T>.close() Void
```

Semantics match native Go channels on the Go target:

- `send` blocks until the value is accepted or buffered.
- `recv` blocks until a value is available or the channel is closed and drained.
- `recv` returns `some(value)` for a received value and `none` after close-and-drain.
- `close` closes the channel for sending.
- Sending on a closed channel panics.
- Closing an already closed channel panics.

The sender owns close discipline. Ard does not add an `is_open` API because such a check would be race-prone.

### `ard/async::start`

Ard does not add a concurrency keyword for spawning work. Spawning uses a module-shaped compiler intrinsic:

```ard
use ard/async

async::start(fn() {
  work()
})
```

Signature:

```ard
fn start(task: fn() Void) Void
```

`async::start` schedules `task` to run concurrently/asynchronously according to the target's concurrency model. On the Go target it lowers to a goroutine:

```go
go task()
```

It is fire-and-forget: no join handle, no returned value, and no automatic panic/error recovery. Programs should use channels for communication and synchronization. Future targets must either define equivalent scheduling semantics or reject `async::start` with a clear target diagnostic.

## Consequences

- Channels become a first-class, typed concurrency primitive in Ard.
- The Go target can lower channels and async start directly to native `chan T` and goroutines without FFI wrappers.
- Directional channel types let Ard express Go's receive-only and send-only channel contracts.
- The API remains module-shaped and avoids adding Go-specific concurrency syntax to Ard.
- `send` and `close` preserve native panic behavior instead of converting failures to `Bool` or `Result`.
- Closure is observed through `recv() -> T?`, aligning with Ard's `Maybe` semantics.
- Targets that cannot support channels or async scheduling must reject these features explicitly.

## Related

- `docs/adrs/0032-select-on-channels.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
