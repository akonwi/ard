---
title: Error Handling with ard/result
description: Work with success and error values using the ard/result module.
---

The `ard/result` module provides functions for working with results that can be either a success value or an error. The `Result` type represents the outcome of an operation that may fail.

:::note
The `ard/result` module is a prelude module. It is automatically imported and aliased as `Result` in all programs, allowing methods to be accessed with the `Result::` namespace (e.g., `Result::ok()`, `Result::err()`).
:::

The result module provides:
- **Success values** with `Result::ok()`
- **Error values** with `Result::err()`
- **Type safety** for error handling without exceptions
- **Result syntax sugar** with the `!` operator (e.g., `Int!Str` is equivalent to `Result<Int, Str>`)

```ard
use ard/result
use ard/io

fn divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("Cannot divide by zero")
  } else {
    Result::ok(a / b)
  }
}

fn main() {
  match divide(10, 2) {
    ok(result) => io::print(result.to_str()),
    err(error) => io::print("Error: {error}")
  }
}
```

## API

### `fn ok(val: $T) Result<$T, $E>`

Create a successful Result containing the given value.

```ard
use ard/result

let result: Int!Str = Result::ok(42)
```

### `fn err(err: $E) Result<$T, $E>`

Create a failed Result containing the given error.

```ard
use ard/result

let result: Int!Str = Result::err("Something went wrong")
```

## Result Type Methods

All `Result` types have the following methods:

### `fn is_ok() Bool`

Check if the Result is a success.

```ard
use ard/result

let result: Int!Str = Result::ok(42)
if result.is_ok() {
  // success
}
```

### `fn is_err() Bool`

Check if the Result is an error.

```ard
use ard/result

let result: Int!Str = Result::err("failed")
if result.is_err() {
  // error
}
```

### `fn or(default: $T) $T`

Get the success value, or return a default if the Result is an error.

```ard
use ard/result

let result: Int!Str = Result::err("failed")
let value = result.or(0)  // 0
```

### `fn expect(message: Str) $T`

Get the success value, or panic with a message if the Result is an error.

```ard
use ard/result

let result: Int!Str = Result::ok(42)
let value = result.expect("Expected success")  // 42
```

### `fn map(with: fn($T) $U) $U!$E`

Transform the `ok` value with a function that **returns a plain value**. The result is automatically wrapped in `ok(...)`. If the Result is an error, the callback is not called and the error passes through unchanged.

Use `map` when the transformation itself cannot fail.

```ard
let result: Int!Str = Result::ok(21)
let doubled = result.map(fn(v) { v * 2 })
let value = doubled.or(0) // 42

// Errors pass through untouched
let err: Int!Str = Result::err("bad")
err.map(fn(v) { v * 2 }).is_err() // true
```

### `fn map_err(with: fn($E) $F) $T!$F`

Transform the `err` value while preserving the same success type. The counterpart to `map` for the error side.

```ard
let result: Int!Str = Result::err("bad")
let sized = result.map_err(fn(err) { err.size() })
sized.is_err() // true (type is Int!Int)
```

### `fn and_then(with: fn($T) $U!$E) $U!$E`

Chain operations that **return a Result themselves** (also known as `flat_map` in other languages). Unlike `map`, the callback is responsible for wrapping its return value in `ok(...)` or `err(...)`. This lets the callback itself decide whether to succeed or fail.

Use `and_then` when the next step can produce its own error.

```ard
fn ensure_even(num: Int) Int!Str {
  match num % 2 == 0 {
    true => Result::ok(num),
    false => Result::err("not even"),
  }
}

let res: Int!Str = Result::ok(20)
let checked = res.and_then(ensure_even)
checked.is_ok() // true

// The callback can fail, unlike map:
let odd: Int!Str = Result::ok(21)
odd.and_then(ensure_even).is_err() // true — "not even"
```

You can provide explicit type arguments to guide inference when needed:

```ard
let as_text = res.and_then<Str>(fn(v) { Result::ok("{v}") })
```

## Pattern Matching with Result

Use `match` expressions to handle both success and error cases:

```ard
use ard/result
use ard/io

fn main() {
  let result: Int!Str = Result::ok(42)
  
  match result {
    ok(value) => io::print("Success: {value.to_str()}"),
    err(error) => io::print("Error: {error}")
  }
}
```

## Result Syntax Sugar

Results can be written in two equivalent ways:

```ard
// Verbose form
fn divide_verbose(a: Int, b: Int) Result<Int, Str> { ... }

// Concise form using ! syntax
fn divide_concise(a: Int, b: Int) Int!Str { ... }
```

Both forms are identical. The `!` syntax is more readable for simple result types.

## Error Propagation with try

Use the `try` keyword to propagate errors to the caller:

```ard
use ard/result

fn divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("Cannot divide by zero")
  } else {
    Result::ok(a / b)
  }
}

fn add_and_divide(a: Int, b: Int, divisor: Int) Int!Str {
  let sum = a + b
  let result = try divide(sum, divisor)
  Result::ok(result + 10)
}

fn main() {
  match add_and_divide(5, 5, 2) {
    ok(value) => io::print(value.to_str()),
    err(error) => io::print(error)
  }
}
```

The `try` keyword unwraps the Result. If it's an error, the entire function returns that error immediately.

## Examples

### Simple Error Handling

```ard
use ard/result
use ard/io

fn main() {
  let result: Int!Str = Result::ok(42)
  
  match result {
    ok(value) => io::print(value.to_str()),
    err(_) => io::print("Operation failed")
  }
}
```

### Propagate Errors

```ard
use ard/result

fn parse_age(age_str: Str) Int!Str {
  match Int::from_str(age_str) {
    age => {
      if age < 0 {
        Result::err("Age cannot be negative")
      } else {
        Result::ok(age)
      }
    },
    _ => Result::err("Invalid integer")
  }
}

fn validate_age(age: Int) Void!Str {
  if age < 18 {
    Result::err("Must be 18 or older")
  } else {
    Result::ok(Void)
  }
}

fn process_user(age_str: Str) Void!Str {
  let age = try parse_age(age_str)
  try validate_age(age)
  Result::ok(Void)
}
```

### Use Defaults on Error

```ard
use ard/result

fn main() {
  let result: Int!Str = Result::err("parsing failed")
  let value = result.or(0)
  
  // value is 0
}
```

### Create Helper Functions

```ard
use ard/result

fn safe_divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("Division by zero")
  } else {
    Result::ok(a / b)
  }
}

fn main() {
  let r1 = safe_divide(10, 2)   // Success
  let r2 = safe_divide(10, 0)   // Error
}
```
