---
title: Concurrent Execution with ard/async
description: Start goroutines with the ard/async module.
---

The `ard/async` module provides the concurrency primitive Ard cannot express on
its own: starting concurrent work. Everything else — waiting for completion,
returning a result, joining a set of tasks — is ordinary Ard built on
[channels](/stdlib/channel/).

```ard
use ard/async
use go:fmt

fn main() {
  let done = Chan::new<Bool>()

  async::start(fn() {
    fmt::Println("running on a goroutine")
    done.send(true)
  })

  done.recv()
}
```

## Functions

### `fn start(do: fn() Void) Void`

Runs `do` concurrently on a new goroutine in the Go target. It is
**fire-and-forget**: `start` returns immediately and gives you no handle.
Coordinate completion or results with a [channel](/stdlib/channel/).

```ard
use ard/async

fn main() {
  let done = Chan::new<Bool>()

  async::start(fn() {
    // ... work ...
    done.send(true)
  })

  done.recv() // wait for the goroutine to finish
}
```

Spawned closures follow Go's concurrency semantics: they capture variables by
reference and there is no isolation rule. Coordinate shared state through
channels; data races are your responsibility, exactly as in Go.

## See also

- [channels](/stdlib/channel/) — typed channels for coordinating goroutines.
- [Async Programming](/advanced/async/) — patterns for results, fan-in, and `select`.
