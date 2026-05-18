---
title: JSON Serialization with ard/json
description: Parse JSON into typed Ard values and encode Ard values as JSON strings.
---

The `ard/json` module provides typed JSON serialization for Ard values.

Use `json::parse<T>` when you know the target type at compile time. It validates that the target type can be represented as JSON and returns either the typed value or a string error.

Use `json::encode` to convert Ard values into JSON strings.

:::note
For ad-hoc or partial decoding of dynamic data, `ard/decode` is still available. Prefer `ard/json` when you want to parse a complete JSON value directly into a struct, list, map, or scalar.
:::

```ard
use ard/json

struct Todo {
  id: Int,
  title: Str,
}

fn main() Str!Str {
  let todo = try json::parse<Todo>("\{\"id\":1,\"title\":\"Ship docs\"\}")
  json::encode(todo)
}
```

## API

### `fn parse(input: Str) $T!Str`

Parse a JSON string into the requested Ard type. Returns a result containing the typed value or an error string if the JSON is invalid or does not match the target type.

Supported target types:

- **Primitives**: `Str`, `Int`, `Float`, `Bool`
- **Collections**: lists and maps with `Str` keys, such as `[Todo]` and `[Str:Int]`
- **Structs**: JSON objects with fields matching the struct field names
- **Nullable types**: `T?`, where `null` and missing nullable struct fields decode as `none`
- **Dynamic**: untyped JSON data

Unsupported JSON shapes, such as non-`Str` map keys or functions, are rejected at compile time.

```ard
use ard/json

struct Person {
  name: Str,
  age: Int,
}

let person = json::parse<Person>("\{\"name\":\"Alice\",\"age\":30\}").expect("valid JSON")
```

### `fn encode(value: $T) Str!Str`

Encode a JSON-compatible Ard value as a JSON string. Returns a result containing the JSON string or an error string if encoding fails.

Supported values include:

- **Primitives**: `Str`, `Int`, `Float`, `Bool`
- **Collections**: lists and maps with `Str` keys
- **Structs**: encoded as objects with named fields
- **Nullable types**: encoded as `null` when `none`
- **Dynamic**: encoded according to the contained value

```ard
use ard/json

let json_text = json::encode(42).expect("encode")
```

## Examples

### Parse a Struct

```ard
use ard/json
use ard/io

struct Person {
  name: Str,
  age: Int,
}

fn main() {
  let person = json::parse<Person>("\{\"name\":\"Alice\",\"age\":30\}").expect("parse")
  io::print("{person.name} is {person.age}")
}
```

### Parse Lists

```ard
use ard/json

let numbers = json::parse<[Int]>("[1,2,3]").expect("parse")
```

### Parse Maps

```ard
use ard/json

let scores = json::parse<[Str:Int]>("\{\"Alice\":95,\"Bob\":87\}").expect("parse")
```

### Parse Nullable Fields

```ard
use ard/json
use ard/maybe

struct User {
  name: Str,
  email: Str?,
}

let user = json::parse<User>("\{\"name\":\"Alice\"\}").expect("parse")
let fallback = user.email.or("unknown@example.com")
```

### Encode Structs

```ard
use ard/json
use ard/io

struct Person {
  name: Str,
  age: Int,
}

fn main() {
  let person = Person{name: "Alice", age: 30}
  let text = json::encode(person).expect("encode")
  io::print(text) // {"age":30,"name":"Alice"}
}
```

### Encode Lists

```ard
use ard/json

let text = json::encode([1, 2, 3, 4, 5]).expect("encode") // [1,2,3,4,5]
```

### Encode Maps

```ard
use ard/json

mut scores: [Str:Int] = [:]
scores.set("Alice", 95)
scores.set("Bob", 87)

let text = json::encode(scores).expect("encode")
```

### Encode Nullable Values

```ard
use ard/json
use ard/maybe

let some_value: Int? = maybe::some(42)
let none_value: Int? = maybe::none()

let some_json = json::encode(some_value).expect("encode") // 42
let none_json = json::encode(none_value).expect("encode") // null
```
