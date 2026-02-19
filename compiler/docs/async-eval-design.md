# Async Eval with Result Types - Design

## Overview

Enhance `async::eval()` to support concurrent operations that return results, with proper type safety through generic `Fiber<T>` types.

## Current State

- `async::start(fn() Void) Fiber` - Spawns concurrent fiber, returns handle with `.join()` method
- Fibers enforce isolation rules (no mutable variable capture)

## Proposed Changes

### 1. Update `Fiber` to be Generic

Define `Fiber<$T>` parameterized by the result type:

```ard
struct Fiber<$T> {
  wg: Dynamic
  result: Dynamic  // Stores the computed result
}
```

### 2. Update `Fiber<T>` Methods

```ard
impl Fiber<$T> {
  fn join() Void {
    // Wait for fiber to complete, discard result
    wait_for(@wg)
  }
  
  fn get() $T {
    // Wait for fiber to complete, return the result
    wait_for(@wg)
    @result as $T
  }
}
```

### 3. New `async::eval()` Signature

```ard
fn eval(do: fn() $T) Fiber<$T>
```

Spawns a concurrent fiber that executes the closure and stores its result. Returns a `Fiber<T>` handle for synchronization and result retrieval.

### 4. New Module-Level `join()` Function

```ard
fn join(fibers: [Fiber]) Void
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

## Implementation Plan

1. Modify `Fiber` struct in `std_lib/async.ard` to support generic type parameter
2. Update `Fiber` methods to include `get()` returning the generic type
3. Change `async::eval()` signature to return `Fiber<$T>`
4. Add module-level `join()` function to `async::eval()` in std_lib.go
5. Update checker validation for `async::eval()` to handle the new signature
6. Update VM execution to store and return results properly
7. Add tests for result retrieval and isolation rules
8. Update documentation in `website/src/content/docs/advanced/async.md`

## Notes

- Result storage needs to be type-safe at runtime (likely using `Dynamic` internally with proper casting)
- The `join()` function simplifies waiting on multiple fibers of different types
- This design maintains backward compatibility with `async::start()` which returns `Fiber` (implicitly `Fiber<Void>`)
