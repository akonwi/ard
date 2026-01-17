---
title: JSON Encoding with ard/json
description: Convert Ard values to JSON strings using the ard/json module.
---

The `ard/json` module provides functions for encoding Ard values into JSON format.

The json module provides:
- **Generic encoding** to convert any type to JSON
- **Automatic handling** of structs, lists, maps, and nullable types
- **Error handling** with Result types for encoding failures

```ard
use ard/json

fn main() {
  let data = ["name": "Alice", "age": 30]
  let json = json::encode(data).expect("Failed to encode")
  io::print(json)
}
```

## API

### `fn encode(value: $T) Str!Str`

Encode any value as a JSON string. Returns a result containing the JSON string or an error message if encoding fails.

The function automatically handles:
- **Primitives**: Str, Int, Float, Bool
- **Collections**: Lists and maps
- **Structs**: Objects with named fields
- **Enums**: Encoded as their numeric values
- **Nullable types**: Encoded as null when none
- **Results**: The ok or err value is encoded

```ard
use ard/json

let json = json::encode(42).expect("Failed to encode")
```

## Examples

### Encode Primitives

```ard
use ard/json
use ard/io

fn main() {
  io::print(json::encode("hello").expect(""))  // "hello"
  io::print(json::encode(42).expect(""))       // 42
  io::print(json::encode(3.14).expect(""))     // 3.14
  io::print(json::encode(true).expect(""))     // true
}
```

### Encode Structs

```ard
use ard/json
use ard/io

struct Person {
  name: Str,
  age: Int
}

fn main() {
  let person = Person { name: "Alice", age: 30 }
  let json = json::encode(person).expect("Failed to encode")
  io::print(json)  // {"name":"Alice","age":30}
}
```

### Encode Lists

```ard
use ard/json
use ard/io

fn main() {
  let numbers = [1, 2, 3, 4, 5]
  let json = json::encode(numbers).expect("Failed to encode")
  io::print(json)  // [1,2,3,4,5]
}
```

### Encode Maps

```ard
use ard/json
use ard/io

fn main() {
  mut scores: [Str:Int] = [:]
  scores.set("Alice", 95)
  scores.set("Bob", 87)
  
  let json = json::encode(scores).expect("Failed to encode")
  io::print(json)  // {"Alice":95,"Bob":87}
}
```

### Encode Nullable Values

```ard
use ard/json
use ard/io
use ard/maybe

fn main() {
  let some_value: Int? = maybe::some(42)
  let none_value: Int? = maybe::none()
  
  io::print(json::encode(some_value).expect(""))  // 42
  io::print(json::encode(none_value).expect(""))  // null
}
```

### Encode Complex Structures

```ard
use ard/json
use ard/io
use ard/maybe

struct Address {
  street: Str,
  city: Str
}

struct User {
  name: Str,
  email: Str?,
  address: Address?
}

fn main() {
  let user = User {
    name: "Alice",
    email: maybe::some("alice@example.com"),
    address: maybe::some(Address {
      street: "123 Main St",
      city: "Portland"
    })
  }
  
  let json = json::encode(user).expect("Failed to encode")
  io::print(json)
}
```
