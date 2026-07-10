---
title: ard/testing
description: Assertion helpers for Ard test functions.
---

The `ard/testing` module provides a small set of helpers for `test fn` declarations. Each helper returns `Void!Str`, so tests can use `try` to stop at the first failure.

```ard
use ard/testing

test fn arithmetic() Void!Str {
  try testing::assert(1 + 1 == 2, "addition should work")
  testing::pass()
}
```

## API

### `pass() Void!Str`

Return a successful test result.

```ard
use ard/testing

test fn example() Void!Str {
  testing::pass()
}
```

### `fail(message: Str) Void!Str`

Return a failed test result with `message`.

```ard
use ard/testing

test fn example() Void!Str {
  testing::fail("expected a value")
}
```

### `assert(condition: Bool, message: Str) Void!Str`

Return success when `condition` is true, otherwise fail with `message`.

```ard
use ard/testing

test fn example() Void!Str {
  try testing::assert("hello".size() == 5, "string size should be 5")
  testing::pass()
}
```
