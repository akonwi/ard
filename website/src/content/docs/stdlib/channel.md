---
title: Channels
description: Send typed values between goroutines with native channels.
---

The built-in channel API provides typed channels for communicating between
goroutines. A channel lowers to a native Go `chan T`. Use channels to coordinate
goroutines started with [`ard/async`](/stdlib/async/) — waiting for completion,
passing results, fan-in — without sharing mutable state.

```ard
use ard/async
use go:fmt

fn main() {
  let results = Chan::new<Int>()

  async::start(fn() {
    results.send(42)
  })

  let value = results.recv() // some(42)
  fmt::Println("{value.or(0)}")
}
```

## Creating a channel

### `Chan::new<$T>(capacity: Int?) Chan<$T>`

Creates a channel carrying values of type `$T`. Omit `capacity` for an
unbuffered synchronous channel where each send blocks until a receiver is ready.
Pass a positive capacity to create a buffered channel that holds that many values
before sends block.

```ard
let unbuffered = Chan::new<Str>()
let buffered = Chan::new<Int>(8)
```

## Operations on `Chan<$T>`

Channels are reference-like handles; `send`, `recv`, and `close` do not require a
`mut` binding.

### `Chan<$T>.send(value: $T) Void`

Blocks until the value is accepted by a receiver or buffered. **Sending on a
closed channel panics** (native Go semantics), so the sending side owns closing.

### `Chan<$T>.recv() $T?`

Blocks until a value is available, or the channel is closed and drained. Returns
`some(value)` for a received value and `none` once the channel is closed and
empty — so `recv() == none` is the closed-and-drained signal. Handle it with an
ordinary `match`:

```ard
match ch.recv() {
  value => use(value),
  _ => stop(), // channel closed and drained
}
```

### `Chan<$T>.close() Void`

Closes the channel for sending. Close from the sending side. **Closing an
already-closed channel panics.** A receiver loop drains remaining values, then
sees `none`:

```ard
use ard/async

fn main() {
  let jobs = Chan::new<Int>(2)

  async::start(fn() {
    jobs.send(1)
    jobs.send(2)
    jobs.close()
  })

  mut total = 0
  mut draining = true
  while draining {
    match jobs.recv() {
      n => { total = total + n },
      _ => { draining = false },
    }
  }
}
```

## Directional channels

A bidirectional `Chan<$T>` can be narrowed to a one-directional view, letting an
API express whether a caller may send or receive:

- `Receiver<$T>` — receive-only (`recv`).
- `Sender<$T>` — send-only (`send`, `close`).

### `Chan<$T>.receiver() Receiver<$T>`
### `Chan<$T>.sender() Sender<$T>`

```ard
let ch = Chan::new<Int>()
let rx = ch.receiver() // Receiver<Int>
let tx = ch.sender()   // Sender<Int>
```

Narrowing is explicit and one-way: there is no implicit conversion, and a
`Receiver`/`Sender` cannot be widened back to a bidirectional `Chan`.

## Selecting across channels

Use the [`select`](/advanced/async/#waiting-on-many-channels-with-select)
expression to wait on several channel operations at once and proceed with
whichever is ready first.
