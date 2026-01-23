---
title: JSON Decoding with ard/decode
description: Parse and decode JSON data into Ard types using the decode module.
---

The `ard/decode` module provides a composable system for decoding JSON and other data into Ard values with detailed error reporting.

The decode module provides:
- **Primitive decoders** for strings, integers, floats, and booleans
- **Composable decoders** that can be combined for complex types
- **Detailed error messages** with path information for debugging
- **JSON parsing** with the `from_json` function

```ard
use ard/decode
use ard/io

fn main() {
  let json_str = "{\"name\": \"Alice\", \"age\": 30}"
  let data = decode::from_json(json_str).expect("Invalid JSON")
  
  let name = decode::run(data, decode::field("name", decode::string)).expect("Failed to decode")
  let age = decode::run(data, decode::field("age", decode::int)).expect("Failed to decode")
  
  io::print("Name: {name}, Age: {age.to_str()}")
}
```

## API

### Basic Decoders

#### `fn string(data: Dynamic) Str![Error]`

Decode a string value from Dynamic data.

```ard
use ard/decode

let data = Dynamic::from("hello")
let str = decode::run(data, decode::string).expect("Failed to decode")
```

#### `fn int(data: Dynamic) Int![Error]`

Decode an integer value from Dynamic data.

```ard
use ard/decode

let data = Dynamic::from(42)
let num = decode::run(data, decode::int).expect("Failed to decode")
```

#### `fn float(data: Dynamic) Float![Error]`

Decode a float value from Dynamic data.

```ard
use ard/decode

let data = Dynamic::from(3.14)
let num = decode::run(data, decode::float).expect("Failed to decode")
```

#### `fn bool(data: Dynamic) Bool![Error]`

Decode a boolean value from Dynamic data.

```ard
use ard/decode

let data = Dynamic::from(true)
let flag = decode::run(data, decode::bool).expect("Failed to decode")
```

### Composite Decoders

#### `fn nullable(decoder: Decoder<$T>) fn(Dynamic) $T?![Error]`

Create a decoder that handles nullable values. Returns `none` for null data, or the result of applying the inner decoder.

```ard
use ard/decode

let string_decoder = decode::nullable(decode::string)
let data = Dynamic::from_void()
let maybe_str = decode::run(data, string_decoder).expect("")  // none
```

#### `fn list(decoder: Decoder<$T>) fn(Dynamic) [$T]![Error]`

Create a decoder for a list of items. Applies the inner decoder to each element and reports detailed errors with array indices.

```ard
use ard/decode

let data = decode::from_json("[1, 2, 3]").expect("Invalid JSON")
let numbers = decode::run(data, decode::list(decode::int)).expect("Failed to decode")
```

#### `fn field(name: Str, with: Decoder<$T>) Decoder<$T>`

Create a decoder for a specific field in an object. Combines the decoder for the field value with path tracking.

```ard
use ard/decode

let data = decode::from_json("{\"name\": \"Alice\"}").expect("Invalid JSON")
let name = decode::run(data, decode::field("name", decode::string)).expect("Failed to decode")
```

#### `fn path(segments: [PathSegment], with: Decoder<$T>) Decoder<$T>`

Create a decoder for a nested path that supports both field names and array indices. The path can mix strings (field names) and integers (array indices) to traverse complex nested structures.

```ard
use ard/decode

// Nested fields only
let data = decode::from_json("{\"user\": {\"profile\": {\"age\": 30}}}").expect("Invalid JSON")
let age = decode::run(data, decode::path(["user", "profile", "age"], decode::int)).expect("Failed to decode")

// Mix fields and array indices
let data2 = decode::from_json("[{\"users\": [{\"name\": \"Alice\"}, {\"name\": \"Bob\"}]}]").expect("Invalid JSON")
let name = decode::run(data2, decode::path([0, "users", 1, "name"], decode::string)).expect("Failed to decode")
```

#### `fn map(key: Decoder<$Key>, value: Decoder<$Value>) Decoder<[$Key:$Value]>`

Create a decoder for maps/objects with custom key and value decoders.

```ard
use ard/decode

let data = decode::from_json("{\"alice\": 30, \"bob\": 25}").expect("Invalid JSON")
let ages = decode::run(data, decode::map(decode::string, decode::int)).expect("Failed to decode")
```

#### `fn one_of(first: Decoder<$T>, others: [Decoder<$T>]) Decoder<$T>`

Create a decoder that tries multiple decoders in sequence, returning the first successful result.

```ard
use ard/decode

let data = Dynamic::from(42)
let flexible_decoder = decode::one_of(decode::string, [decode::int, decode::float])
let result = decode::run(data, flexible_decoder).expect("Failed to decode")
```

#### `fn dynamic(data: Dynamic) Dynamic![Error]`

Decoder that simply maintains the Dynamic value as-is. This can be used as a no-op or simple identity decoder.

```ard
use ard/decode

let data = Dynamic::from("anything")
let result = decode::run(data, decode::dynamic).expect("Failed to decode")
```

### JSON and Utility Functions

#### `fn from_json(json: Str) Dynamic!Str`

Parse a JSON string into a Dynamic value. Returns an error if the JSON is invalid.

```ard
use ard/decode

let data = decode::from_json("{\"name\": \"Alice\"}").expect("Invalid JSON")
```

#### `fn is_void(data: Dynamic) Bool`

Check if a Dynamic value is null/void.

```ard
use ard/decode

let data = Dynamic::from_void()
if decode::is_void(data) {
  // handle null
}
```

#### `fn run(data: Dynamic, decoder: Decoder<$T>) $T![Error]`

Apply a decoder to Dynamic data. This is a shorthand for applying a decoder function directly.

```ard
use ard/decode

let data = Dynamic::from(42)
let num = decode::run(data, decode::int).expect("Failed to decode")
```

### Error Handling

#### `struct Error`

Represents a decoding error with detailed context.

- **`expected: Str`** - What type was expected
- **`found: Str`** - What was actually found
- **`path: [Str]`** - The path to the error location (e.g., ["user", "age"])

The `Error` struct implements `ToString` for readable error messages:

```ard
use ard/decode

let data = Dynamic::from("not_a_number")
match decode::run(data, decode::int) {
  err(errors) => {
    for err in errors {
      io::print(err.to_str())  // "Decode error: expected Int, found "not_a_number""
    }
  },
  ok(_) => {}
}
```

## Examples

### Decode a Simple Object

```ard
use ard/decode
use ard/io

fn main() {
  let json = "{\"name\": \"Alice\", \"age\": 30}"
  let data = decode::from_json(json).expect("Invalid JSON")
  
  let name = decode::run(data, decode::field("name", decode::string)).expect("Failed")
  let age = decode::run(data, decode::field("age", decode::int)).expect("Failed")
  
  io::print("Name: {name}")
  io::print("Age: {age.to_str()}")
}
```

### Decode a List

```ard
use ard/decode
use ard/io

fn main() {
  let json = "[1, 2, 3, 4, 5]"
  let data = decode::from_json(json).expect("Invalid JSON")
  
  let numbers = decode::run(data, decode::list(decode::int)).expect("Failed to decode")
  
  for num in numbers {
    io::print(num.to_str())
  }
}
```

### Decode with Error Details

```ard
use ard/decode
use ard/io

fn main() {
  let json = "{\"user\": {\"name\": \"Alice\", \"age\": \"thirty\"}}"
  let data = decode::from_json(json).expect("Invalid JSON")
  
  let decoder = decode::field("user", 
    decode::field("age", decode::int)
  )
  
  match decode::run(data, decoder) {
    ok(age) => io::print(age.to_str()),
    err(errors) => {
      for error in errors {
        io::print(error.to_str())  // "Decode error: expected Int, found "thirty" at user.age"
      }
    }
  }
}
```

### Decode Nullable Fields

```ard
use ard/decode
use ard/io
use ard/maybe

struct Person {
  name: Str,
  email: Str?
}

fn main() {
  let json1 = "{\"name\": \"Alice\", \"email\": \"alice@example.com\"}"
  let json2 = "{\"name\": \"Bob\", \"email\": null}"
  
  let decoder = decode::field("email", decode::nullable(decode::string))
  
  let data1 = decode::from_json(json1).expect("")
  let email1 = decode::run(data1, decoder).expect("")  // some("alice@example.com")
  
  let data2 = decode::from_json(json2).expect("")
  let email2 = decode::run(data2, decoder).expect("")  // none
}
```

### Flexible Decoding with one_of

```ard
use ard/decode
use ard/io

fn main() {
  // Decoder that accepts either a string or an integer
  let flexible = decode::one_of(decode::string, [decode::int])
  
  let str_data = Dynamic::from("hello")
  let int_data = Dynamic::from(42)
  
  let str_result = decode::run(str_data, flexible).expect("")
  let int_result = decode::run(int_data, flexible).expect("")
}
```
