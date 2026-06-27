# 0019: Use Typed Channels for Fiber Communication

## Status

Accepted

Partially superseded: the lowering is revised by [ADR 0031](0031-go-backend-lowering-contract.md) (channels lower to native `chan T` rather than through FFI), and the channel API shape plus the "no `select` keyword" guidance are superseded by [ADR 0032](0032-select-on-channels.md). The core decision — typed channels for fiber communication — still stands.

## Context

Ard fibers can start concurrent work and retrieve typed results through `async::Fiber<$T>`, but Ard does not yet have a native way for concurrently running fibers to communicate while they are running. Programs must either wait for a final fiber result or route communication through target-specific externs.

This is limiting for Go interop. Go's concurrency model is built around goroutines and channels, and Ard's Go target already maps fibers to Go-style concurrent execution. If Ard code needs to exchange values with Go code, or expose concurrent Ard components to Go companion code, a channel-like type should lower predictably to a Go `chan T` instead of being represented as an ad-hoc FFI handle.

Ard extern type bindings can name concrete host types, but representing a generic host type such as `chan T` would require a templating/substitution convention in extern bindings. Channels are central enough to Ard's fiber model that they should not depend on a general-purpose generic host-type binding syntax.

The design should preserve Ard's type safety, keep synchronization explicit at call sites, and avoid importing all of Go's channel syntax into Ard before the language has a complete concurrency story.

## Decision

Add typed channels as a target-aware Ard concurrency primitive in a nested standard-library module: `ard/async/channel`.

Do not put the channel API directly in `ard/async`, and do not create a top-level `ard/channel` module. The nested module keeps channels grouped with Ard's fiber/concurrency surface while giving callers the concise default qualifier `channel::` after:

```ard
use ard/async/channel
```

Use two Ard types:

```ard
extern type Chan<$T>

struct Channel {
  chan: Chan<$T>,
}
```

`Chan<$T>` is the opaque, compiler-recognized channel handle. It carries values of exactly `$T` and is the type that maps to host channels. On the Go target, `Chan<$T>` lowers directly to `chan T`, where `T` is the Go representation of `$T`. `Chan<$T>` remains public because it is harmless as an opaque type and useful for project and standard-library Go FFI signatures that need to accept or return native Go channel values.

`Channel<$T>` is the Ard-facing wrapper that provides methods and hides raw channel operations from ordinary code. It is a normal Ard struct containing the raw `Chan<$T>` handle, so it can attach methods without making the raw handle itself constructable in Ard source.

Channel construction is intrinsic because it defines the native representation and because a generic Go FFI constructor cannot infer `T` from arguments when `T` appears only in the return type. Public constructors should create a raw `Chan<$T>` through compiler-defined lowering and wrap it in `Channel<$T>`.

Start with a small method-shaped API rather than new channel operators or `select` syntax:

```ard
use ard/async
use ard/async/channel

let ch = channel::new<Int>()
let queue = channel::new<Int>(size: 16)

let sender = async::start(fn() {
  ch.send(42)
  ch.close()
})

let value: Int? = ch.recv()
sender.join()
```

Initial semantics:

- `channel::new<$T>(size: Int?) Channel<$T>` creates a channel. Omitting `size` or passing `none` creates an unbuffered synchronous channel with no queue capacity. Passing `0` also creates an unbuffered channel, matching Go. Passing a positive size creates a buffered, FIFO, queue-backed channel. Passing a negative size falls back to `0` and creates an unbuffered channel.
- For unbuffered channels, `send` and `recv` rendezvous directly: each operation blocks until the opposite side is ready.
- For buffered channels, `send` blocks only when the buffer is full, and `recv` blocks only when the buffer is empty.
- `Channel<$T>.send(value: $T) Bool` blocks until the value is accepted or buffered, and returns `false` if the channel is closed before or during the send.
- `Channel<$T>.recv() $T?` blocks until a value is available or the channel is closed. It returns `some(value)` for received values and `none` after close and drain.
- `Channel<$T>.close() Bool` closes the channel for sending. Closing is explicit and should be done by the sending side. Closing an already-closed channel returns `false`.
- Channel values are reference-like synchronization handles. Sending, receiving, and closing do not require a mutable channel binding. A read-only `Channel<$T>` binding may be captured by fiber closures and used for communication.

The public channel methods can delegate to private extern functions implemented by Go FFI companion code that uses native Go channel operations for send, receive, and close:

```ard
private extern fn chan_send(ch: Chan<$T>, value: $T) Bool = "ChannelSend"
private extern fn chan_recv(ch: Chan<$T>) $T? = "ChannelRecv"
private extern fn chan_close(ch: Chan<$T>) Bool = "ChannelClose"

impl Channel {
  fn send(value: $T) Bool {
    chan_send(self.chan, value)
  }

  fn recv() $T? {
    chan_recv(self.chan)
  }

  fn close() Bool {
    chan_close(self.chan)
  }
}
```

The constructor remains the only intrinsic channel operation because it must produce the native `Chan<$T>` representation. Ordinary send, receive, and close operations should stay in the standard-library/FFI layer as private extern calls implemented in real Go code, not as direct compiler/backend lowering. Go FFI implementations for `send` and `close` should adapt closed-channel panics into `false` results.

Do not add `Channel<$T>.is_open() Bool` to the initial API. Native Go channels do not provide a reliable open-state query. Any `is_open()` result would be immediately stale in concurrent code, and a wrapper-maintained flag could be bypassed by raw `Chan<$T>` values crossing Go FFI. Callers should attempt `send` or `close` and handle the returned `Bool`; receivers should use `recv() == none` as the closed-and-drained signal.

Other targets must make support explicit. Since JavaScript targets do not currently support Ard's fiber model, `ard/async/channel` is Go-target-only until a separate target compatibility decision defines equivalent semantics.

Do not add directional channel types in the initial implementation, such as send-only, publish-only, or receive-only channel views. The initial API uses bidirectional `Channel<$T>` and raw `Chan<$T>` values. Directional views can be added later if real Ard or Go-interop use cases need them.

Do not add Go-style `select`, cancellation, cancellation callbacks, close callbacks, non-blocking send/receive, or async iteration in the initial implementation. Single-channel closure handling is already expressible by matching on `recv()`'s `Maybe` result:

```ard
match ch.recv() {
  value => handle(value),
  _ => break,
}
```

If Ard later adds Go-select-like multiplexing across multiple channel operations, prefer a channel-aware `match` form instead of introducing a new `select` keyword. Such a form must not behave like an ordinary sequential `match`: it must register all channel operations together, block until one is ready, and then run the matching arm. Future cancellation should be a separate signal/token that can participate in that channel-aware match, not a meaning of channel close.

These deferred features can be layered on later once the base channel representation and blocking semantics are stable.

## Consequences

- Ard gains a typed communication primitive for coordinating fibers during execution, not only at `Fiber.get()` boundaries.
- The raw `Chan<$T>` type stays opaque in Ard source, avoiding a misleading constructable struct with phantom fields.
- The ergonomic `Channel<$T>` wrapper can provide methods while preserving a raw native channel type for Go interop.
- Go interop remains simple when signatures use `Chan<$T>`: Ard channels are represented as native Go channels instead of opaque handles.
- The design avoids adding generic host-type templating to extern type bindings solely to express `chan T`.
- The Go backend must lower `Chan<$T>` to the correct concrete `chan T` type and lower channel construction to Go `make`.
- FFI remains the implementation boundary for blocking send/recv/close operations, keeping channel behavior in ordinary Go companion code where possible. Construction stays compiler-defined because it must create the native channel representation, and send/close panics must be adapted into `false` results.
- Blocking behavior is explicit through `send` and `recv` calls, which fits the existing std-lib function/method style.
- `send()` and `close()` make closed-channel state recoverable through a simple Boolean because the only expected failure is that the channel is already closed.
- `recv() $T?` gives Ard a safe closed-and-drained signal without exposing Go's zero-value-plus-ok receive form.
- Omitting `is_open()` avoids a race-prone state-check API; senders handle Boolean send/close results and receivers observe closure through `recv()` returning `none`.
- Channels become part of the checker and backend's target-aware concurrency model; unsupported targets must reject them clearly.
- The first version is intentionally small. Directional channel views, channel-aware `match` multiplexing, cancellation tokens, and more expressive coordination patterns require future design work rather than being implied by the existence of `Channel<$T>`.

## Related

- `docs/adrs/0003-use-generic-fibers-for-async-eval.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `compiler/std_lib/async.ard`
- `compiler/std_lib/async/channel.ard`
