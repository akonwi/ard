---
title: Integer Operations with ard/int
description: Convert and work with integers using Ard's int module.
---

The `ard/int` module provides functions for converting and parsing integers.

:::note
The `ard/int` module is a prelude module. It is automatically imported and aliased as `Int` in all programs, allowing methods to be accessed with the `Int::` namespace (e.g., `Int::from_str()`).
:::

The int module provides:
- **String parsing** to convert strings to integers
- **Error handling** with Maybe types for failed conversions

```ard
use ard/int

fn main() {
  let num = Int::from_str("42").or(0)
}
```

## API

### `fn from_str(str: Str) Int?`

Parse a string into an integer. Returns a `Maybe` type - `some` if parsing succeeds, `none` if it fails.

```ard
use ard/int

let num = Int::from_str("42")
match num {
  val => io::print(val.to_str()),
  _ => io::print("Failed to parse")
}
```

## Examples

### Parse Integer from User Input

```ard
use ard/int
use ard/io

fn main() {
  io::print("Enter a number: ")
  let input = io::read_line().expect("Failed to read")
  
  match Int::from_str(input) {
    val => io::print("You entered: {val.to_str()}"),
    _ => io::print("Invalid integer")
  }
}
```

### Use with Default Value

```ard
use ard/int

fn main() {
  let age = Int::from_str("thirty").or(18)
  // age is 18 since parsing failed
}
```

### Validate User Input

```ard
use ard/int
use ard/io

fn main() {
  io::print("Enter age (1-120): ")
  let input = io::read_line().expect("Failed to read")
  
  match Int::from_str(input) {
    age => {
      if age >= 1 and age <= 120 {
        io::print("Valid age: {age.to_str()}")
      } else {
        io::print("Age out of valid range")
      }
    },
    _ => io::print("Invalid integer")
  }
}
```
