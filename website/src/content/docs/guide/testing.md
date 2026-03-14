---
title: Testing
description: Write and run tests for your Ard programs using the built-in test framework.
---

Ard has a built-in testing framework. Tests are written as regular Ard functions marked with the `test` keyword and executed with the `ard test` command.

## Writing Tests

A test function is declared with `test fn`. Every test function must:

- Take no parameters
- Return `Void!Str`

A test passes when it returns `Result::ok(())` and fails when it returns `Result::err(...)` with an error message.

```ard
use ard/testing

test fn addition_works() Void!Str {
  try testing::assert(1 + 1 == 2, "Expected 1 + 1 to equal 2")
  testing::pass()
}

test fn greeting_format() Void!Str {
  let name = "World"
  let greeting = "Hello, {name}!"
  try testing::assert(greeting == "Hello, World!", "Expected greeting format")
  testing::pass()
}
```

## Test Helpers

The `ard/testing` module provides assertion helpers that return `Void!Str`, so you can use `try` to propagate failures:

| Function | Description |
|----------|-------------|
| `pass()` | Returns a successful test result (`Result::ok(())`) |
| `fail(message)` | Always fails with the given message |
| `assert(condition, message)` | Fails with the message if the condition is false |

```ard
use ard/testing

test fn test_assertions() Void!Str {
  try testing::assert(true, "should be true")
  try testing::assert(2 * 3 == 6, "multiplication should work")
  try testing::assert("a" == "a", "string equality should work")
  testing::pass()
}
```

Since test helpers return `Void!Str`, you use `try` to short-circuit on the first failure — just like normal error propagation in Ard. End each test with `testing::pass()` to signal success.

## Where to Put Tests

Tests can be placed in two locations:

### Co-located tests

Write test functions directly in the same file as the code they test. This is useful for unit-style tests that exercise internal logic.

```
my_project/
  ard.toml
  math.ard       ← contains both `fn add(...)` and `test fn test_add()`
```

### The `/test` directory

Place test files under a `/test` directory at the project root. These files import and test the public API of your modules — they cannot access `private` symbols.

```
my_project/
  ard.toml
  math.ard
  test/
    math_test.ard  ← imports my_project/math and tests its public API
```

## Running Tests

Use the `ard test` command to discover and run tests:

```bash
# Run all tests in the current project
ard test

# Run tests in a specific file
ard test math.ard

# Run tests in a directory
ard test src/

# Filter tests by name
ard test --filter math

# Stop on first failure
ard test --fail-fast
```

## Test Output

Each test produces one of three results:

- **PASS** — the test returned `Result::ok(())`
- **FAIL** — the test returned `Result::err(message)`
- **PANIC** — the test crashed with an unexpected runtime error

```
PASS  math::test_add
FAIL  math::test_subtract
  Expected values to be equal
PANIC  math::test_divide
  division by zero

2 passed; 1 failed; 1 panicked
```

Panics indicate a more severe problem than a normal assertion failure — they represent unexpected crashes rather than expected test failures.

## Restrictions

Test functions have a few restrictions enforced by the compiler:

- Must be top-level declarations (not nested inside structs or other functions)
- Must not take parameters
- Must not be generic
- Must return `Void!Str`

```ard
// ✗ Compile error: test functions must not take parameters
test fn bad_test(x: Int) Void!Str { Result::ok(()) }

// ✗ Compile error: test functions must not be generic
test fn bad_generic<$T>() Void!Str { Result::ok(()) }

// ✓ Valid test function
test fn good_test() Void!Str {
  testing::pass()
}
```
