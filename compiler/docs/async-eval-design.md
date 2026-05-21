# Async Eval with Result Types

Status: implemented.

## Overview

`async::eval()` supports concurrent operations that return results, with type safety through generic `Fiber<T>` values.

## Current API

- `async::start(fn() Void) Fiber<Void>` spawns a concurrent fiber and returns a handle with `.join()`.
- `async::eval(fn() $T) Fiber<$T>` spawns a concurrent fiber and stores the function result.
- `Fiber<$T>.get() $T` waits for completion and returns the result.
- Fibers enforce isolation rules (no mutable variable capture).

## Implemented shape

### 1. Generic `Fiber`

`Fiber` carries a generic result field:

```ard
struct Fiber {
  wg: WaitGroup,
  result: $T,
}
```

### 2. `Fiber<T>` Methods

```ard
impl Fiber<$T> {
  fn join() Void {
    // Wait for fiber to complete, discard result
    wait_for(self.wg)
  }
  
  fn get() $T {
    // Wait for fiber to complete, return the result
    wait_for(self.wg)
    get_result(self.wg, self.result)
  }
}
```

### 3. `async::eval()` Signature

```ard
fn eval(do: fn() $T) Fiber<$T>
```

Spawns a concurrent fiber that executes the closure and stores its result. Returns a `Fiber<T>` handle for synchronization and result retrieval.

### 4. Module-Level `join()` Function

```ard
fn join(fibers: [Fiber<$T>]) Void
```

Wait for multiple fibers to complete without individual `.join()` calls. Accepts a list of any fiber types.

## Usage Example

```ard
use ard/async

fn expensive_operation(n: Int) Int {
  // ... compute something ...
  n * 2
}

fn main() {
  let fiber1 = async::eval(fn() { expensive_operation(5) })
  let fiber2 = async::eval(fn() { expensive_operation(10) })
  
  // Wait for all to complete
  async::join([fiber1, fiber2])
  
  // Get results
  let result1 = fiber1.get()  // Returns 10
  let result2 = fiber2.get()  // Returns 20
}
```

## Isolation Rules

Both `async::start()` and `async::eval()` enforce the same isolation rules:
- ✅ Can access read-only variables from outer scope
- ❌ Cannot access mutable variables from outer scope

These rules are enforced at compile-time.

## Completed implementation points

- `std_lib/async.ard` defines `Fiber` with a generic result field.
- `Fiber.get()` waits and returns `$T`.
- `async::eval()` returns `Fiber<$T>`.
- Module-level `async::join()` waits on a list of fibers.
- Go FFI support stores and retrieves fiber results through the async runtime helpers.
- Checker validation covers the fiber isolation rules.

## Notes

- `join()` is intentionally for synchronization; use `.get()` when the result value is needed.
- `async::start()` remains the `Void`-returning convenience API and returns `Fiber<Void>`.
