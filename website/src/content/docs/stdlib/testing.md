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
      Result::ok(())
    }
    _ => testing::fail("Expected a value")
  }
}
```

### `assert(condition: Bool, message: Str?) Void!Str`

Fails with the message if the condition is false. If no message is provided, the default message is `"Assertion failed"`.

```ard
use ard/testing

test fn test_assert() Void!Str {
  try testing::assert(1 + 1 == 2, "math works")
  try testing::assert(true)
  Result::ok(())
}
```

### `equal(actual: $T, expected: $T) Void!Str`

Fails if the two values are not equal.

```ard
use ard/testing

test fn test_equal() Void!Str {
  try testing::equal(2 + 2, 4)
  try testing::equal("hello", "hello")
  Result::ok(())
}
```

### `not_equal(actual: $T, expected: $T) Void!Str`

Fails if the two values are equal.

```ard
use ard/testing

test fn test_not_equal() Void!Str {
  try testing::not_equal(1, 2)
  try testing::not_equal("a", "b")
  Result::ok(())
}
```

## Usage Pattern

Since all helpers return `Void!Str`, the idiomatic pattern is to `try` each assertion and end with `testing::pass()`:

```ard
use ard/testing

test fn example() Void!Str {
  try testing::assert(condition(), "check one")
  try testing::equal(compute(), expected)
  testing::pass()
}
```

This allows the first failing assertion to short-circuit the test, reporting its error message.
