---
title: Dynamic Values with ard/dynamic
description: Create and work with dynamically-typed values using the ard/dynamic module.
---

The `ard/dynamic` module provides functions for creating `Dynamic` values, which can represent any Ard value. Dynamic types are useful for working with untyped data, such as JSON parsing results or FFI data.

:::note
The `ard/dynamic` module is a prelude module. It is automatically imported and aliased as `Dynamic` in all programs, allowing methods to be accessed with the `Dynamic::` namespace (e.g., `Dynamic::from_str()`, `Dynamic::object()`).
:::

The dynamic module provides:
- **Primitive conversions** to wrap basic types as Dynamic
- **Collection builders** for creating dynamic lists and objects
- **Generic transformation** with the `from()` function

```ard
use ard/dynamic
use ard/json

fn main() {
  // Create a dynamic object
  let data = Dynamic::object([
    "name": Dynamic::from("Alice"),
    "age": Dynamic::from(30)
  ])
  
  let json = json::encode(data).expect("Failed to encode")
  io::print(json)
}
```

## API

### Creating Dynamic Values

#### `fn from_str(val: Str) Dynamic`

Convert a string to a Dynamic value.

```ard
use ard/dynamic

let dyn = Dynamic::from_str("hello")
```

#### `fn from_int(val: Int) Dynamic`

Convert an integer to a Dynamic value.

```ard
use ard/dynamic

let dyn = Dynamic::from_int(42)
```

#### `fn from_float(val: Float) Dynamic`

Convert a float to a Dynamic value.

```ard
use ard/dynamic

let dyn = Dynamic::from_float(3.14)
```

#### `fn from_bool(val: Bool) Dynamic`

Convert a boolean to a Dynamic value.

```ard
use ard/dynamic

let dyn = Dynamic::from_bool(true)
```

#### `fn from_void() Dynamic`

Create a Dynamic null value.

```ard
use ard/dynamic

let dyn = Dynamic::from_void()
```

#### `fn from_list(list: [Dynamic]) Dynamic`

Create a Dynamic array from a list of Dynamic values.

```ard
use ard/dynamic

let arr = Dynamic::from_list([
  Dynamic::from_int(1),
  Dynamic::from_int(2)
])
```

#### `fn object(from: [Str:Dynamic]) Dynamic`

Create a Dynamic object from a map of string keys to Dynamic values.

```ard
use ard/dynamic

let obj = Dynamic::object([
  "key": Dynamic::from_str("value")
])
```

### Generic Conversion

#### `fn from(primitive: Primitive) Dynamic`

Convert a primitive value (Str, Int, Float, Bool, or Void) to Dynamic. This function uses pattern matching to handle different types.

```ard
use ard/dynamic

let s: Str | Int = "hello"
let dyn = Dynamic::from(s)
```

#### `fn list(from: [$T], of: fn($T) Dynamic) Dynamic`

Convert a list of typed values to a Dynamic array by applying a transformation function to each element.

```ard
use ard/dynamic

let nums = [1, 2, 3]
let dyn = Dynamic::list(nums, fn(n: Int) Dynamic { 
  Dynamic::from_int(n * 2) 
})
```

## Examples

### Create a Dynamic Object

```ard
use ard/dynamic
use ard/json

fn main() {
  let person = Dynamic::object([
    "name": Dynamic::from_str("Alice"),
    "age": Dynamic::from_int(30),
    "active": Dynamic::from_bool(true)
  ])
  
  let json = json::encode(person).expect("Failed to encode")
  io::print(json)  // {"name":"Alice","age":30,"active":true}
}
```

### Convert Typed List to Dynamic

```ard
use ard/dynamic
use ard/json

fn main() {
  let ages = [25, 30, 35]
  let dynamic_ages = Dynamic::list(ages, fn(age: Int) Dynamic {
    Dynamic::from_int(age)
  })
  
  let json = json::encode(dynamic_ages).expect("Failed to encode")
  io::print(json)  // [25,30,35]
}
```

### Build Complex Dynamic Structure

```ard
use ard/dynamic
use ard/json

fn main() {
  let users = Dynamic::from_list([
    Dynamic::object(["name": Dynamic::from_str("Alice"), "id": Dynamic::from_int(1)]),
    Dynamic::object(["name": Dynamic::from_str("Bob"), "id": Dynamic::from_int(2)])
  ])
  
  let json = json::encode(users).expect("Failed to encode")
  io::print(json)
}
```
