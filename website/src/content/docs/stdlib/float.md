---
title: Float Operations with ard/float
description: Convert and manipulate floating-point numbers using Ard's float module.
---

The `ard/float` module provides functions for working with floating-point numbers, including conversion and mathematical operations.

:::note
The `ard/float` module is a prelude module. It is automatically imported and aliased as `Float` in all programs, allowing methods to be accessed with the `Float::` namespace (e.g., `Float::from_str()`, `Float::floor()`).
:::

The float module provides:
- **String conversion** to parse floats from strings
- **Type conversion** from integers to floats
- **Mathematical operations** like floor

```ard
use ard/float

fn main() {
  let f = Float::from_str("3.14").or(0.0)
  let floored = Float::floor(f)
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
  io::print(result.to_str())  // 2.0
}
```

### Convert and Floor

```ard
use ard/float

fn main() {
  let temp_c = Float::from_int(25)
  // Scale to Fahrenheit and floor
  let fahrenheit = (temp_c * 1.8) + 32.0
  let floored = Float::floor(fahrenheit)
  io::print(floored.to_str())
}
```
