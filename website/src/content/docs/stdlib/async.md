---
title: Concurrent Execution with ard/async
description: Run code concurrently using fibers with the ard/async module.
---

The `ard/async` module provides functions for concurrent execution using fibers, which are lightweight concurrent execution contexts backed by Go's goroutines.

The async module provides:
- **Sleep operations** to pause execution for a specified duration
- **Fiber spawning** with `async::start()` for background tasks
- **Concurrent evaluation** with `async::eval()` for computing values concurrently
- **Fiber synchronization** with `.join()` method and `async::join()` function
- **Result retrieval** via `.get()` method on fibers
- **Compile-time isolation** to prevent data races

```ard
use ard/async
use ard/io

fn main() {
  // Start a background task
  let fiber = async::start(fn() {
    async::sleep(100000000)  // Sleep 100ms
    io::print("Background task complete")
  })
  
  io::print("Main task continues")
  fiber.join()  // Wait for background task
}
```

## API

### `fn sleep(ns: Int) Void`

Sleep for a specified duration in nanoseconds.

```ard
use ard/async

async::sleep(1000000000)  // Sleep for 1 second (1 billion nanoseconds)
```

### `fn start(do: fn() Void) Fiber`

Spawn a concurrent fiber that executes the given closure. Returns a `Fiber` handle for synchronization.

The closure runs in an isolated scope and cannot access mutable variables from the parent scope, only read-only ones.

```ard
use ard/async
use ard/io

fn main() {
  let config = "settings"  // Read-only, accessible in fiber
  
  let fiber = async::start(fn() {
    io::print("Fiber running with: {config}")
  })
  
  fiber.join()  // Wait for completion
}
```

### `fn eval(do: fn() $T) Fiber`

Spawn a concurrent fiber that executes the given closure and returns a `Fiber` handle to retrieve the result.

Like `async::start()`, the closure runs in an isolated scope. The generic return type `$T` adapts to whatever the closure returns.

```ard
use ard/async
use ard/io

fn expensive_computation(n: Int) Int {
  async::sleep(100000000)
  n * 2
}

fn main() {
  let fiber = async::eval(fn() { expensive_computation(5) })
  let result = fiber.get()  // Wait and get the result
  io::print(result.to_str())  // Prints "10"
}
```

### `fn join(fibers: [Fiber]) Void`

Wait for multiple fibers to complete.

```ard
use ard/async

fn main() {
  let fiber1 = async::eval(fn() { 10 + 20 })
  let fiber2 = async::eval(fn() { 30 + 40 })
  
  // Wait for both to complete
  async::join([fiber1, fiber2])
  
  // Get results
  let sum1 = fiber1.get()  // 30
  let sum2 = fiber2.get()  // 70
}
```

## Fiber Type

### `struct Fiber`

A handle to a concurrent fiber with methods for synchronization and result retrieval.

#### Methods

##### `fn join() Void`

Wait for the fiber to complete. Does not return a result.

```ard
use ard/async

let fiber = async::start(fn() {
  // do work
})

fiber.join()  // Block until fiber completes
```

##### `fn get() $T`

Wait for the fiber to complete and return its result. Non-producing fibers (those created with `::start()`, will return `Void` here)

```ard
use ard/async

let fiber = async::eval(fn() { 42 })
let result = fiber.get()  // Wait and get result
```

## Isolation Rules

Both `async::start()` and `async::eval()` enforce compile-time isolation rules to prevent data races:

- **Read-only variables** from parent scopes are accessible
- **Mutable variables** from parent scopes are NOT accessible

```ard
use ard/async

fn main() {
  let readonly = "safe to access"
  mut mutable = "unsafe to access"
  
  async::start(fn() {
    io::print(readonly)   // ✅ OK
    io::print(mutable)    // ❌ Error: mutable variable not accessible
  })
}
```

This design ensures safe concurrent execution by preventing race conditions at compile-time.

## Examples

### Parallel Computation

```ard
use ard/async
use ard/io

fn fib(n: Int) Int {
  if n <= 1 {
    n
  } else {
    fib(n - 1) + fib(n - 2)
  }
}

fn main() {
  let fib1 = async::eval(fn() { fib(20) })
  let fib2 = async::eval(fn() { fib(21) })
  
  async::join([fib1, fib2])
  
  let r1 = fib1.get()
  let r2 = fib2.get()
  
  io::print("fib(20) = {r1.to_str()}")
  io::print("fib(21) = {r2.to_str()}")
}
```

### Background Worker

```ard
use ard/async
use ard/io

fn main() {
  async::start(fn() {
    mut count = 0
    while count < 5 {
      io::print("Worker tick {count.to_str()}")
      async::sleep(100000000)  // 100ms
      count = count + 1
    }
  })
  
  io::print("Main task complete")
}
```
