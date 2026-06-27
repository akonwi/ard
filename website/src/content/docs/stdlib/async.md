---
title: Concurrent Execution with ard/async
description: Start goroutines and sleep with the ard/async module.
---

The `ard/async` module provides the two concurrency primitives Ard cannot
express on its own: starting a goroutine and sleeping. Everything else —
waiting for completion, returning a result, joining a set of tasks — is
ordinary Ard built on [channels](/stdlib/channel/).

```ard
use ard/async
use ard/io

fn main() {
  async::start(fn() {
    io::print("running on a goroutine")
  })

  async::sleep(1_000_000) // give it a moment (1ms)
}
```

## Functions

### `fn start(do: fn() Void)`

Runs `do` concurrently on a new goroutine. It is **fire-and-forget**: `start`
returns immediately and gives you no handle. Coordinate completion or results
with a [channel](/stdlib/channel/).

```ard
use ard/async
use ard/channel

fn main() {
  let done = channel::new<Bool>(0)

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

### `fn sleep(nanoseconds: Int)`

Blocks the current goroutine for the given number of nanoseconds.

```ard
async::sleep(1_000_000_000) // 1 second
```

There are no duration helpers in the standard library. Use a plain nanosecond
count, or import Go's `time` constants for readable values — `time::Second`,
`time::Millisecond`, and friends are durations in nanoseconds and convert
directly to `Int`:

```ard
use ard/async
use go:time

fn main() {
  async::sleep(time::Second)      // 1 second
  async::sleep(time::Millisecond * 250)
}
```

## See also

- [`ard/channel`](/stdlib/channel/) — typed channels for coordinating goroutines.
- [Async Programming](/advanced/async/) — patterns for results, fan-in, and `select`.
