---
title: "WIP: Async Programming"
description: Learn about asynchronous programming in Ard using fibers and the async module.
---

## Overview

Ard takes inspiration from Go and Rust for async execution.
Go's goroutines power the underlying async implementation.
From Rust, Ard adopts safety guardrails for managing shared memory between execution contexts.

## The `ard/async` Module

Async functionality is provided through the `ard/async` standard library module:

## Basic Sleep

From the current (main) fiber, a program can sleep for a duration in milliseconds:

```ard
use ard/async
use ard/io

fn main() {
  io::print("hello...")
  async::sleep(1000)  // Sleep for 1 second
  io::print("world!")
}
```

## No `async`/`await` Keywords

Note how there's no `async`, `await`, or `go` keyword.
Ard avoids JavaScript's problem of infectious `async` functions where async spreads throughout the codebase.
"Awaiting" is usually implicit when calling obviously blocking operations like `sleep()`.

## Fibers

Ard uses **fibers** for concurrent execution contexts instead of OS threads.
Currently, all fibers are goroutines under the hood, meaning they are "green" threads managed by the Go runtime.

> 💡 This design choice keeps the possibility of OS-level threads open without naming conflicts.

## Concurrent Execution

Use `async::start()` to start running code concurrently (a coroutine):

```ard
use ard/async
use ard/io

fn main() {
  io::print("1")

  async::start(fn() {
    io::print("2")
  })

  io::print("3")
}
```

The output of this program is not guaranteed to have the numbers in order (could be "1", "3", "2" or "1", "2", "3") because there's no control over when the async callback runs.

## Waiting for Fibers

The `async::start()` function returns a fiber handle that provides a `.wait()` method:

```ard
use ard/async
use ard/io

fn main() {
  io::print("1")

  let fiber = async::start(fn() {
    async::sleep(5000)  // Sleep for 5 seconds
    io::print("2")
  })

  fiber.wait()  // Wait for the fiber to complete
  io::print("3")
}
```

Now the output will always be:
```
1
2
3
```

With a 5-second delay between "1" and "2".
