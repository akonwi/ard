---
title: "Async Programming"
description: Learn about asynchronous programming in Ard using the async module.
---

## Overview

Ard takes inspiration from Go and Rust for async execution:
- Go's goroutines power the underlying async implementation
- From Rust, Ard adopts safety guardrails for managing concurrent execution

## Fibers

Ard uses **fibers** for concurrent execution contexts. Currently, all fibers are goroutines under the hood, meaning they are "green" threads managed by the Go runtime.

> 💡 This design choice keeps the possibility of OS-level threads open without naming conflicts.

Fiber scopes can safely access readonly data from parent scopes and mutable access is prohibited to guard against race conditions.

## The `ard/async` Module

Async functionality is provided through the `ard/async` standard library module.

### Basic Sleep

From the current (main) fiber, a program can sleep for a duration in milliseconds:

```ard
use ard/async
use ard/io

fn main() {
  io::print("hello...")
  async::sleep(100)  // Sleep for 100ns
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
    async::sleep(5)  // Sleep for 5ns
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

## Scope Access

Fibers can access **read-only variables** from parent scopes, but cannot access mutable variables:

```ard
use ard/async
use ard/io

fn main() {
  let value = 42      // Immutable - accessible in fiber
  mut count = 0       // Mutable - NOT accessible in fiber

  async::start(fn() {
    io::print(value)  // ✅ Works! Read-only access is safe
    count += 1        // ❌ Error: mutable variables are isolated
  })
}
```

This design provides safe concurrent execution by:
- **Allowing** access to readonly variables
- **Preventing** access to mutable references, which could cause race conditions

### Practical Example

For background tasks that need external data, use module-level functions or reload data within the fiber:

```ard
use ard/async
use ard/duration
use maestro/config
use maestro/db

fn main() {
  // Start background worker
  async::start(fn() {
    let conn = db::connect()

    while true {
      // Do work...
      conn.query("SELECT * FROM foo WHERE id = @id").run(["id": 1])
      // ...
      async::sleep(duration::from_minutes(5))
    }
  })

  // Continue with main program...
}
```

## Design Philosophy

Each fiber runs in its own concurrent context with:
- **Read-only access** to immutable variables from parent scopes
- **Complete isolation** from mutable state
