---
title: Float Operations with ard/float
description: Convert and manipulate floating-point numbers using Ard's float module.
---

The `ard/float` module provides functions for working with floating-point numbers, including conversion, formatting, and mathematical operations.

:::note
The `ard/float` module is a prelude module. It is automatically imported and aliased as `Float` in all programs, allowing methods to be accessed with the `Float::` namespace (e.g., `Float::from_str()`, `Float::floor()`, `Float::ceil()`, `Float::round()`).
:::

:::tip
Use `Float::format(value, decimals)` when output needs a fixed number of decimal places. The general `.to_str()` conversion is best for simple/debug output.
:::

The float module provides:
- **String conversion** to parse floats from strings
- **Formatting** with a portable fixed-decimal contract
- **Type conversion** from integers to floats
- **Mathematical operations** like floor, ceil, and round

```ard
use ard/float

fn main() {
  let f = Float::from_str("3.14").or(0.0)
  let floored = Float::floor(f)
  let ceiled = Float::ceil(f)
  let rounded = Float::round(f)
  let label = Float::format(rounded, 2)  // "3.00"
}
```

## API

### `fn from_str(string: Str) Float?`

Parse a string into a float. Returns a `Maybe` type - `some` if parsing succeeds, `none` if it fails.

```ard
use ard/float
use ard/maybe

let f = Float::from_str("3.14")
match f {
  val => io::print(val.to_str()),
  _ => io::print("Failed to parse")
}
```

### `fn from_int(int: Int) Float`

Convert an integer to a float.

```ard
use ard/float

let f = Float::from_int(42)  // 42.0
```

### `fn floor(float: Float) Float`

Return the largest integer less than or equal to the given float.

```ard
use ard/float

Float::floor(3.7)  // 3.0
Float::floor(3.2)  // 3.0
Float::floor(-2.5)  // -3.0
```

### `fn ceil(float: Float) Float`

Return the smallest integer greater than or equal to the given float.

```ard
use ard/float

Float::ceil(3.7)  // 4.0
Float::ceil(3.0)  // 3.0
Float::ceil(-2.5)  // -2.0
```

### `fn round(float: Float) Float`

Round to the nearest integer. Halfway cases round away from zero.

```ard
use ard/float

Float::round(3.2)   // 3.0
Float::round(3.5)   // 4.0
Float::round(-3.5)  // -4.0
```

### `fn format(value: Float, decimals: Int) Str`

Format a float as fixed-point decimal text with exactly `decimals` digits after the decimal point. Negative `decimals` values are treated as `0`.

```ard
use ard/float

Float::format(1.0, 2)      // "1.00"
Float::format(3.14159, 3)  // "3.142"
Float::format(42.0, 0)     // "42"
Float::format(42.7, -1)    // "43"
```

Halfway cases round to the nearest even final digit, so `Float::format(2.5, 0)` returns `"2"` and `Float::format(3.5, 0)` returns `"4"`.

## Examples

### Parse Float from User Input

```ard
use ard/float
use ard/io

fn main() {
  io::print("Enter a number: ")
  let input = io::read_line().expect("Failed to read")
  
  match Float::from_str(input) {
    val => io::print("You entered: {val.to_str()}"),
    _ => io::print("Invalid float")
  }
}
```

### Perform Float Calculations

```ard
use ard/float

fn main() {
  let x = Float::from_int(5)
  let y = Float::from_str("2.5").or(0.0)
  let result = Float::floor(x / y)
  let rounded = Float::round(x / y)
  let rounded_up = Float::ceil(x / y)
  io::print(result.to_str())  // 2.0
  io::print(rounded.to_str())  // 2.0
  io::print(rounded_up.to_str())  // 2.0
}
```

### Convert, Floor, and Ceil

```ard
use ard/float

fn main() {
  let temp_c = Float::from_int(25)
  // Scale to Fahrenheit and round down/up
  let fahrenheit = (temp_c * 1.8) + 32.0
  let floored = Float::floor(fahrenheit)
  let ceiled = Float::ceil(fahrenheit)
  io::print(floored.to_str())
  io::print(ceiled.to_str())
}
```
