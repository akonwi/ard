---
title: Channels with ard/async/channel
description: Send values between fibers with typed channels.
---

The `ard/async/channel` module provides typed channels for communicating between fibers. Channels are currently backed by Go channels and are available on the Go target.

Use channels when one fiber needs to send values to another fiber without sharing mutable state.

```ard
use ard/async
use ard/async/channel
use ard/io

fn main() {
  let ch = channel::new<Int>()

  let sender = async::start(fn() {
    ch.send(42)
    ch.close()
  })

  let value = ch.recv().or(0)
  sender.join()

  io::print(value)
}
```

## API

### `extern type Chan<$T>`

The raw native channel type. Most Ard code should use `Channel<$T>` instead.

Project FFI functions may use `channel::Chan<T>` when they need to receive or return a raw Go channel.

```ard
use ard/async/channel

extern fn observe(ch: channel::Chan<Int>) Int = "Observe"
```

### `struct Channel<$T>`

A typed channel wrapper.

Fields:

- `chan: Chan<$T>` — the raw native channel.

Most user code creates a channel with `channel::new<T>()` and then uses the methods below.

### `fn new<$T>(size: Int?) Channel<$T>`

Create a channel for values of type `$T`.

- If `size` is omitted or `none`, the channel is unbuffered.
- If `size` is positive, the channel is buffered with that capacity.
- Non-positive sizes fall back to an unbuffered channel.

```ard
use ard/async/channel

let unbuffered = channel::new<Int>()
let buffered = channel::new<Str>(size: 10)
```

The type argument is required so the compiler knows what values can move through the channel.

### `fn send(value: $T) Bool`

Send a value to the channel.

Returns `true` if the send succeeds. Returns `false` if the channel has already been closed.

```ard
let ch = channel::new<Int>(size: 1)
let sent = ch.send(7)
```

For unbuffered channels, `send` blocks until another fiber receives the value.

### `fn recv() $T?`

Receive the next value from the channel.

Returns:

- `some(value)` when a value is received.
- `none` when the channel is closed and drained.

```ard
let ch = channel::new<Int>(size: 1)
ch.send(7)
ch.close()

let value = ch.recv().or(0) // 7
let done = ch.recv().is_none() // true
```

### `fn close() Bool`

Close the channel.

Returns `true` if this call closed the channel. Returns `false` if the channel was already closed.

```ard
let ch = channel::new<Int>()
let closed = ch.close()
let closed_again = ch.close() // false
```

## Working with fibers

Channels are most useful with `ard/async` fibers.

```ard
use ard/async
use ard/async/channel
use ard/io

fn main() {
  let ch = channel::new<Str>()

  let producer = async::start(fn() {
    ch.send("ready")
    ch.close()
  })

  match ch.recv() {
    message => io::print(message),
    _ => io::print("channel closed"),
  }

  producer.join()
}
```

## Type safety

Channels are generic. A `Channel<Int>` only accepts `Int` values and `recv()` returns `Int?`.

```ard
let ints = channel::new<Int>()
ints.send(1)      // OK
ints.send("one")  // Error: expected Int
```

If you refer to the `Channel` type directly, include its type argument:

```ard
use ard/async/channel

// If the Ard project is named `demo`, project Go FFI types are qualified
// with the generated project package name.
extern type RawEvent = "demo.Event"
extern fn events() channel::Channel<RawEvent> = "Events"
```

## Go FFI notes

When an Ard extern function is declared as returning `channel::Channel<T>`, the Go FFI function can return the raw Go channel (`chan T`). The Go backend wraps that raw channel into Ard's `Channel<T>` wrapper before Ard code calls methods like `.recv()`.

```ard
use ard/async/channel

// If the Ard project is named `demo`, project Go FFI types are qualified
// with the generated project package name.
extern type RawEvent = "demo.Event"
extern fn events() channel::Channel<RawEvent> = "Events"
```

```go
package ffi

type Event struct{}

func Events() chan Event {
    return make(chan Event)
}
```

Use `channel::Chan<T>` in Ard extern signatures when you explicitly want the raw native channel type.
