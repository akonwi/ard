---
title: "Async Programming"
description: Write concurrent Ard programs with goroutines and channels.
---

## Overview

Ard's concurrency model is Go's: lightweight goroutines that communicate over
typed channels. There are no `async`/`await` keywords and no promises. Instead,
two small primitives compose into everything else:

- [`ard/async`](/stdlib/async/) starts goroutines.
- Built-in channels (`Chan<T>`, `Receiver<T>`, and `Sender<T>`) pass typed values between them.

Waiting for completion, returning results, joining a set of tasks, fan-in, and
timeouts are all written in ordinary Ard on top of these — not baked into the
language.

## Starting goroutines

`async::start` runs a function on a new goroutine. It is **fire-and-forget**: it
returns immediately and gives you no handle.

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

Because `start` returns nothing, you coordinate with a channel rather than a
return value. Spawned closures follow Go's semantics: they capture by reference,
and there is no isolation rule — shared state is coordinated through channels and
data races are your responsibility.

## Coordinating with channels

A channel is a typed conduit between goroutines. `send` blocks until a value is
taken (or buffered); `recv` blocks until a value arrives or the channel is
closed.

### Waiting for completion

```ard
use ard/async
use go:fmt

fn main() {
  let done = Chan::new<Bool>()

  async::start(fn() {
    fmt::Println("working")
    done.send(true)
  })

  done.recv() // blocks until the goroutine signals
  fmt::Println("finished")
}
```

### Returning a result

A goroutine "returns" by sending its result on a channel. This is the pattern
that replaces a result-returning task: start the work, then receive its value.

```ard
use ard/async

fn compute() Int {
  // ... expensive work ...
  42
}

fn main() Int {
  let result = Chan::new<Int>()

  async::start(fn() {
    result.send(compute())
  })

  result.recv().or(0)
}
```

### Joining many goroutines (fan-in)

To wait on several goroutines, have each send to the same channel and receive
once per goroutine:

```ard
use ard/async
use ard/list as List

fn main() Int {
  let inputs = [1, 2, 3, 4]
  let results = Chan::new<Int>(inputs.size())

  for n in inputs {
    async::start(fn() {
      results.send(n * n)
    })
  }

  mut total = 0
  for _ in inputs {
    total = total + results.recv().or(0)
  }
  total // 30
}
```

### Closing and draining

The sending side closes a channel when it is done producing. A receiver loops
until `recv()` returns `none`, the closed-and-drained signal:

```ard
use ard/async

fn main() Int {
  let jobs = Chan::new<Int>()

  async::start(fn() {
    jobs.send(1)
    jobs.send(2)
    jobs.send(3)
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
  total // 6
}
```

## Directional channels

An API can express direction by narrowing a bidirectional `Chan<$T>` to a
receive-only `Receiver<$T>` or a send-only `Sender<$T>`. Narrowing is explicit
and one-way:

```ard

fn produce(out: Sender<Int>) {
  out.send(1)
  out.close()
}

fn main() {
  let ch = Chan::new<Int>()
  produce(ch.sender())
  // ch.recv() ...
}
```

## Waiting on many channels with `select`

`select` waits on several channel operations at once and runs the arm whose
operation is ready first. Unlike a sequence of `recv` calls, it registers every
arm together and blocks until one can proceed. If several are ready, one is
chosen at random (Go's fairness). It is an expression, so it can produce a value.

```ard
select {
  let job = jobs.recv() => match job {
    j => run(j),   // received a value
    _ => drain(),  // jobs closed (recv() == none)
  },
  results.send(value) => recorded(), // a send arm
  timeout.recv() => give_up(),       // receive and discard
  _ => idle(),                       // non-blocking default
}
```

- **`let name = ch.recv() => body`** binds `name: $T?`; a closed channel surfaces
  as `none`, handled with a normal `match`.
- **`ch.recv() => body`** receives and discards the value.
- **`ch.send(x) => body`** sends an in-scope value; with a `_` arm it is a
  non-blocking try-send.
- **`_ => body`** runs when no other arm is ready, making the whole `select`
  non-blocking.

### Timeouts

Go's `time::After` returns a channel that fires after a delay, which composes
naturally as a `select` arm:

```ard
use go:time

fn main() Int {
  let work = Chan::new<Int>()
  // ... a goroutine may eventually send on `work` ...

  select {
    let v = work.recv() => v.or(0),
    time::After(time::Second).recv() => -1, // timed out
  }
}
```

## Design philosophy

Ard deliberately keeps async tiny. The only thing the language must provide is a
way to *start* a goroutine, because there is no `go` statement to write in
userland. Everything a richer async library would offer — futures, joins,
wait-groups, structured concurrency — is expressible as plain Ard over channels,
so it lives in libraries and programs rather than in the runtime. The runtime
itself defines no async type; `start` is a compiler-backed intrinsic, and channels lower
to native `chan T`.
