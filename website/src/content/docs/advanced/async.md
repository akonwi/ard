---
title: "Async Programming"
description: Learn about asynchronous programming in Ard using fibers and the async module.
---

## Overview

Ard takes inspiration from Go and Rust for async execution:
- Go's goroutines power the underlying async implementation
- From Rust, Ard adopts safety guardrails for managing concurrent execution

## Fibers

Ard uses **fibers** for concurrent execution contexts. Currently, all fibers are goroutines under the hood, meaning they are "green" threads managed by the Go runtime.

> 💡 This design choice keeps the possibility of OS-level threads open without naming conflicts.

Each fiber runs as an **independent program** with its own runtime and module loading. This design ensures complete isolation between concurrent executions, eliminating entire classes of concurrency bugs.

## The `ard/async` Module

Async functionality is provided through the `ard/async` standard library module.

### Basic Sleep

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

### No `async`/`await` Keywords

Note how there's no `async`, `await`, or `go` keyword. Ard avoids JavaScript's problem of infectious `async` functions where async spreads throughout the codebase.

## Starting Fibers

Use `async::start()` to run code concurrently:

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

The output of this program is not guaranteed to have the numbers in order (could be "1", "3", "2" or "1", "2", "3") because there's no control over when the fiber executes.

## Waiting for Fibers

The `async::start()` function returns a `Fiber` handle that provides a `.join()` method:

```ard
use ard/async
use ard/io

fn main() {
  io::print("1")

  let fiber = async::start(fn() {
    async::sleep(5000)  // Sleep for 5 seconds
    io::print("2")
  })

  fiber.join()  // Wait for the fiber to complete
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

## Scope Isolation

Fibers run as completely isolated programs. Functions passed to `async::start()` **cannot access variables from the outer scope**:

```ard
use ard/async

fn main() {
  let value = 42

  async::start(fn() {
    // Error: cannot access `value` - it's not in scope
    io::print(value)
  })
}
```

This restriction prevents data races and ensures safe concurrent execution. Each fiber must use:
- Its own local variables
- Module imports (which are loaded fresh for each fiber)
- Module-level functions and constants

### Practical Example

For background tasks that need external data, use module-level functions or reload data within the fiber:

```ard
use ard/async
use maestro/config
use maestro/db

fn main() {
  // Start background worker
  async::start(fn() {
    // Each fiber creates its own database connection
    let conn = db::connect()

    while true {
      // Do work...
      async::sleep(60000)
    }
  })

  // Continue with main program...
}
```

## Design Philosophy

Think of `async::start()` as spawning an **independent program** that runs concurrently. Each fiber:
- Loads its own copy of all modules
- Has its own isolated scope and state
- Cannot share memory with other fibers
- Must manage its own resources (connections, file handles, etc.)
