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

From the current (main) fiber, a program can sleep for a duration in nanoseconds:

```ard
use ard/async
use ard/io

fn main() {
  io::print("hello...")
  async::sleep(1000000000)  // Sleep for 1 second (1 billion nanoseconds)
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
    async::sleep(100000000)  // Sleep for 100 milliseconds
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

With a 100 millisecond delay between "1" and "2".

## Evaluating Functions with Results

The `async::eval()` function executes a closure concurrently and returns a `Fiber` handle. Like `async::start()`, it creates a concurrent fiber, but the `Fiber` allows you to retrieve the computed result:

```ard
use ard/async
use ard/io

fn main() {
  let fiber = async::eval(fn() {
    5 + 10
  })

  let result = fiber.get()  // Wait for result
  io::print(result.to_str())  // Prints "15"
}
```

The generic return type `$T` adapts to whatever the function returns:

```ard
use ard/async
use ard/io

fn main() {
  let sum_fiber = async::eval(fn() {
    10 + 20
  })
  let sum = sum_fiber.get()
  io::print(sum.to_str())  // Prints "30"

  let message_fiber = async::eval(fn() {
    "Hello from eval"
  })
  let message = message_fiber.get()
  io::print(message)  // Prints "Hello from eval"
}
```

### Multiple Concurrent Operations

You can run multiple concurrent operations and wait for all of them to complete:

```ard
use ard/async
use ard/io

fn expensive_computation(n: Int) Int {
  async::sleep(100000000)  // Sleep 100ms
  n * 2
}

fn main() {
  let fiber1 = async::eval(fn() { expensive_computation(5) })
  let fiber2 = async::eval(fn() { expensive_computation(10) })
  
  // Both computations run concurrently
  async::join([fiber1, fiber2])
  
  // Get the results
  let result1 = fiber1.get()  // 10
  let result2 = fiber2.get()  // 20
}
```

### Differences from `async::start()`

| Feature | `async::start()` | `async::eval()` |
|---------|------------------|-----------------|
| Execution | Concurrent (returns immediately) | Concurrent (returns immediately) |
| Return Value | `Fiber` handle with `.join()` method | `Fiber` handle with `.join()` and `.get()` methods |
| Result Access | Not available | Retrieved via `.get()` with type safety |
| Use Case | Background tasks | Computing values concurrently with result retrieval |

## Scope Access

Both `async::start()` and `async::eval()` enforce the same isolation rules. Functions passed to either can access **read-only variables** from parent scopes, but cannot access mutable variables:

```ard
use ard/async
use ard/io

fn main() {
  let value = 42      // Immutable - accessible in both start() and eval()
  mut count = 0       // Mutable - NOT accessible in either start() or eval()

  async::start(fn() {
    io::print(value)  // ✅ Works! Read-only access is safe
    count += 1        // ❌ Error: mutable variables are isolated
  })

  let result = async::eval(fn() {
    value * 2         // ✅ Works! Read-only access is safe
    count += 1        // ❌ Error: mutable variables are isolated
  })
}
```

This design provides safe concurrent execution by:
- **Allowing** access to readonly variables
- **Preventing** access to mutable references, which could cause race conditions

Both approaches enforce these rules at compile-time, preventing data races before the program runs.

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

## API Reference

For a complete API reference of the `ard/async` module, see the [Async Module Reference](/stdlib/async).
