---
title: Types
description: Overview of Ard's type system including built-in types, type unions, and nullable values.
---

## Built-in Types

### Primitive Types

```ard
let text: Str = "Hello, World!"
let number: Int = 42
let decimal: Float = 3.14
let flag: Bool = true
let byte: Byte = Byte::from_int(255).expect("byte")
let rune: Rune = 'é'
let newline: Rune = '\n'
```

`Byte` represents an unsigned 8-bit value (`0..255`). `Rune` represents one Unicode scalar value. Single-quoted rune literals make scalar comparisons concise, such as `ch == '/'` while iterating a string. Rune literals support escapes like `'\n'`, `'\x00'`, and `'\u0080'`. Convert text explicitly with `"text".bytes()`, `"text".runes()`, `Str::from_bytes(bytes)`, and `Str::from_runes(runes)`.

### Collection Types

```ard
// Lists
let numbers: [Int] = [1, 2, 3, 4, 5]
let names: [Str] = ["Alice", "Bob", "Charlie"]

// Maps
let scores: [Str:Int] = ["Alice": 95, "Bob": 87]
let config: [Int:Str] = [0: "zero", 1: "one", 2: "two"]
```

### Void

`Void` is another built-in primitive that represents non-existence. It's rarely used in Ard except as a placeholder to signal an impossibility (Similar to `never` in Typescript).

## Type Inference

The compiler can infer types from context:

```ard
let count = 42
let items = [1, 2, 3]
let person = ["name": "Alice", "age": 30]
```

## Type Unions

Type unions allow a value to be one of several types:

```ard
type Printable = Str | Int
type Value = Int | Float | Str

let item: Printable = "Hello"
let data: Value = 42
```

### Working with Type Unions

Use match expressions to handle different types in a union:

```ard
type Content = Str | Int | Bool

fn describe(value: Content) Str {
  match value {
    Str => "Text: {it}"
    Int => "Number: {it.to_str()}"
    Bool => "Flag: {it.to_str()}"
  }
}

let items: [Content] = ["hello", 42, true]
for item in items {
  io::print(describe(item))
}
```

The `it` variable is automatically bound to the matched value.

## Nullable Types (Maybe)

Use the `?` suffix after a type to declare the possibility of it not being present.

```ard
use ard/maybe

mut maybe_name: Str? = maybe::none()
maybe_name = maybe::some("Alice")
```

When the wrapped type starts with `mut`, group it before adding `?`:

```ard
use ard/maybe

let maybe_ref: (mut Widget)? = maybe::none()
```

### Working with Maybe Types

```ard
// Pattern matching
match maybe_name {
  name => "Hello, {name}!"
  _ => "Hello, stranger!"
}

// Checking presence
if maybe_name.is_some() {
  io::print("Name is present")
}

if maybe_name.is_none() {
  io::print("No name provided")
}

// Providing defaults
let name: Str = maybe_name.or("Anonymous")
```

### Maybe Type Methods

```ard
let maybe_value: Int? = maybe::some(42)

// Check if value is present
let has_value: Bool = maybe_value.is_some()
let is_empty: Bool = maybe_value.is_none()

// Get value or default
let value: Int = maybe_value.or(0)
```

## Generic Syntax

Use a `$` prefix on a type to indicate a generic (unspecfied type).

```ard
fn identity(value: $T) $T {
  value
}
```
