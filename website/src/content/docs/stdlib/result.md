---
title: ard/result
description: Constructors and methods for explicit success-or-error values.
---

Ard represents recoverable errors as values with `Result`, commonly written with `!` syntax:

```ard
fn divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("division by zero")
  } else {
    Result::ok(a / b)
  }
}
```

`Result` is available from the prelude. You can also import `ard/result` when you want the module namespace explicitly.

## Constructors

### `Result::ok(val: $T) $T!$E`

Create a successful result.

```ard
let result: Int!Str = Result::ok(42)
```

### `Result::err(err: $E) $T!$E`

Create a failed result.

```ard
let result: Int!Str = Result::err("failed")
```

## Methods

### `is_ok() Bool`

Return `true` when the result is successful.

```ard
if result.is_ok() {
  // success
}
```

### `is_err() Bool`

Return `true` when the result is an error.

```ard
if result.is_err() {
  // error
}
```

### `expect(message: Str) $T`

Return the success value or fail with `message`.

```ard
let value = result.expect("expected success")
```

### `or(default: $T) $T`

Return the success value or `default`.

```ard
let value = result.or(0)
```

### `map(with: fn($T) $U) $U!$E`

Transform the success value while preserving errors.

```ard
let result: Int!Str = Result::ok(21)
let doubled = result.map(fn(v: Int) Int { v * 2 })
```

### `map_err(with: fn($E) $F) $T!$F`

Transform the error value while preserving success values.

```ard
let result: Int!Str = Result::err("bad")
let sized = result.map_err(fn(err: Str) Int { err.size() })
```

### `and_then(with: fn($T) $U!$E) $U!$E`

Chain another operation that can fail.

```ard
fn ensure_even(num: Int) Int!Str {
  match num % 2 == 0 {
    true => Result::ok(num),
    false => Result::err("not even"),
  }
}

let checked = Result::ok(20).and_then(ensure_even)
```

## Pattern matching

Use `match` to handle both cases:

```ard
match divide(10, 2) {
  ok(value) => value,
  err(message) => 0,
}
```

## Error propagation with `try`

`try` unwraps an `ok` value. If the result is an error, the current function returns that error immediately.

```ard
fn add_and_divide(a: Int, b: Int, divisor: Int) Int!Str {
  let sum = a + b
  let result = try divide(sum, divisor)
  Result::ok(result + 10)
}
```
