---
title: ard/async
description: Start concurrent work on the Go backend.
---

The `ard/async` module provides one compiler-backed operation: starting concurrent work.

Channels are built-in types (`Chan<T>`, `Receiver<T>`, and `Sender<T>`) and are the usual way to coordinate with work started by `async::start`.

```ard
use ard/async
use go:fmt

fn main() {
  let done = Chan::new<Bool>()

  async::start(fn() {
    fmt::Println("running concurrently")
    done.send(true)
  })

  done.recv()
}
```

## API

### `start(do: fn() Void) Void`

Run `do` concurrently. On the Go backend this lowers to a goroutine.

`start` is fire-and-forget: it returns immediately and does not provide a handle. Use channels or other explicit coordination when the caller needs a result or completion signal.

```ard
use ard/async

fn main() {
  let done = Chan::new<Bool>()

  async::start(fn() {
    // work happens concurrently
    done.send(true)
  })

  done.recv()
}
```

Captured variables follow the backend's concurrency rules. On Go, closures capture by reference and Ard does not add data-race protection. Prefer communicating through channels.

## Related built-ins

- `Chan<T>`: bidirectional typed channel.
- `Receiver<T>`: receive-only channel view.
- `Sender<T>`: send-only channel view.
- `Chan::new<T>(capacity: Int?) Chan<T>`: create a channel. Omit the capacity or pass `none` for an unbuffered channel.
