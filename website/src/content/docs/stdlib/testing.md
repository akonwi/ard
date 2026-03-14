---
title: ard/testing
description: Assertion helpers for writing tests in Ard.
---

The `ard/testing` module provides assertion functions for use in `test fn` declarations. All helpers return `Void!Str`, so they integrate naturally with Ard's error propagation via `try`.

## Functions

### `pass() Void!Str`

Returns a successful test result. A shorthand for `Result::ok(())`.

```ard
use ard/testing

test fn test_example() Void!Str {
  try testing::assert(1 + 1 == 2, "math works")
  testing::pass()
}
```

### `fail(message: Str) Void!Str`

Always returns an error with the given message. Useful for marking unreachable branches as failures.

```ard
use ard/testing

test fn test_unreachable() Void!Str {
  let result = some_operation()
  match result {
    value => {
      try testing::assert(value > 0, "expected positive")
      testing::pass()
    }
    _ => testing::fail("Expected a value")
  }
}
```

### `assert(condition: Bool, message: Str) Void!Str`

Fails with the provided message if the condition is false.

```ard
use ard/testing

test fn test_assert() Void!Str {
  try testing::assert(1 + 1 == 2, "math works")
  try testing::assert("hello".size() == 5, "string size should be 5")
  testing::pass()
}
```

## Usage Pattern

Since all helpers return `Void!Str`, the idiomatic pattern is to `try` each assertion and end with `testing::pass()`:

```ard
use ard/testing

test fn example() Void!Str {
  try testing::assert(condition(), "check one")
  try testing::assert(compute() == expected, "compute() should match expected")
  testing::pass()
}
```

This allows the first failing assertion to short-circuit the test, reporting its error message.
