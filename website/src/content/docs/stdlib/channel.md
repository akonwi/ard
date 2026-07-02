---
title: Channels with ard/channel
description: Send typed values between goroutines with native channels.
---

The `ard/channel` module provides typed channels for communicating between
goroutines. A channel lowers to a native Go `chan T`. Use channels to coordinate
goroutines started with [`ard/async`](/stdlib/async/) — waiting for completion,
passing results, fan-in — without sharing mutable state.

```ard
use ard/async
use ard/channel
use ard/io

fn main() {
  let results = channel::new<Int>(0)

  async::start(fn() {
    results.send(42)
  })

  let value = results.recv() // some(42)
  io::print("{value.or(0)}")
}
```

## Creating a channel

### `fn new(capacity: Int) Chan<$T>`

Creates a channel carrying values of type `$T`. `capacity` is the buffer size:
`0` is an unbuffered (synchronous) channel where each send blocks until a
receiver is ready; a positive capacity buffers that many values before sends
block.

```ard
let unbuffered = channel::new<Str>(0)
let buffered = channel::new<Int>(8)
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
use ard/channel

fn main() {
  let jobs = channel::new<Int>(2)

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

### `fn receiver(ch: Chan<$T>) Receiver<$T>`
### `fn sender(ch: Chan<$T>) Sender<$T>`

```ard
let ch = channel::new<Int>(0)
let rx = channel::receiver(ch) // Receiver<Int>
let tx = channel::sender(ch)   // Sender<Int>
```

Narrowing is explicit and one-way: there is no implicit conversion, and a
`Receiver`/`Sender` cannot be widened back to a bidirectional `Chan`.

## Selecting across channels

Use the [`select`](/advanced/async/#waiting-on-many-channels-with-select)
expression to wait on several channel operations at once and proceed with
whichever is ready first.
