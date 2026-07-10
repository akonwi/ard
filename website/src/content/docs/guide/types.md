---
title: Types
description: Ard's built-in types, their Go counterparts, and the types Ard adds on top.
---

Ard's type system has two layers:

1. **Built-in types** that correspond directly to Go types. They follow Ard naming conventions but keep familiar Go semantics.
2. **Ard-native types** that Go does not have, like `Maybe`, `Result`, and type unions. These carry Ard's stricter semantics.

## Built-in Types

These types map directly onto Go:

| Ard | Go | Notes |
| --- | --- | --- |
| `Str` | `string` | Immutable UTF-8 text |
| `Int` | `int` | Platform-sized integer |
| `Float64` | `float64` | Default floating-point type |
| `Bool` | `bool` | |
| `Byte` | `byte` | Unsigned 8-bit value |
| `Rune` | `rune` | One Unicode scalar value |
| `[T]` | `[]T` | List / slice |
| `[T; N]` | `[N]T` | Fixed-size array |
| `[K:V]` | `map[K]V` | Map |
| `Chan<T>` | `chan T` | Typed channel |
| `Any` | `any` | Opaque boxed value |

Sized numeric types are also available when a specific width or signedness is needed: `Int8`, `Int16`, `Int32`, `Int64`, `Uint`, `Uint8`, `Uint16`, `Uint32`, `Uint64`, `Uintptr`, and `Float32`, each corresponding to the same-named Go type. Literals are range-checked at compile time:

```ard
let small: Int8 = 127
let port: Uint16 = 8080
let ratio: Float32 = 1.5
```

For ordinary code, prefer the default `Int` and `Float64`; reach for sized types when interfacing with Go APIs or when the width matters.

### Primitives

```ard
let text: Str = "Hello, World!"
let number: Int = 42
let decimal: Float64 = 3.14
let flag: Bool = true
let byte: Byte = 255
let rune: Rune = 'é'
let newline: Rune = '\n'
```

`Byte` represents an unsigned 8-bit value (`0..255`). `Rune` represents one Unicode scalar value. Single-quoted rune literals make scalar comparisons concise, such as `ch == '/'` while iterating a string. Rune literals support escapes like `'\n'`, `'\x00'`, and `'\u0080'`.

Convert text explicitly with:

```ard
let bytes: [Byte] = "text".bytes()
let runes: [Rune] = "text".runes()
let from_bytes: Str = Str::from(bytes)
let from_runes: Str = Str::from(runes)
```

`Str::from([Byte])` mirrors Go's `string([]byte)` conversion; validate bytes first if your program needs to reject invalid UTF-8.

### Collections

```ard
// Lists
let numbers: [Int] = [1, 2, 3, 4, 5]
let names: [Str] = ["Alice", "Bob", "Charlie"]

// Fixed-size arrays
let rgb: [Byte; 3] = [255, 128, 0]
let empty: [Int; 0] = []

// Maps
let scores: [Str:Int] = ["Alice": 95, "Bob": 87]
let config: [Int:Str] = [0: "zero", 1: "one", 2: "two"]
```

Lists and maps behave like Go slices and maps, with methods like `.size()`, `.push()`, and `.at()` in place of Go's built-in functions. Fixed-size arrays behave like Go arrays: the length is part of the type, so `[Byte; 3]` and `[Byte; 4]` are distinct types. Lists and arrays support `.at()`, which returns a `Maybe` instead of panicking or returning a zero value.

### Any

`Any` is an opaque boxed value, corresponding to Go's `any`. Any Ard value can be assigned to it, but unlike Go there is no type assertion syntax: an `Any` cannot be inspected, called, or unboxed without an explicit API such as [`unsafe::cast`](/stdlib/unsafe/).

```ard
let boxed: Any = 42
```

## Ard-Native Types

These types have no direct Go equivalent. They are where Ard's opinionated semantics live.

### Void

`Void` represents non-existence. It is rarely written except in function signatures that explicitly return no value.

### Nullable Types (Maybe)

Ard has no `nil`. A value that may be absent is declared with the `?` suffix, or formally as `Maybe<T>`, and the compiler requires the absent case to be handled.

```ard
mut maybe_name: Str? = Maybe::new()
maybe_name = Maybe::new("Alice")

let formal: Maybe<Int> = Maybe::new(42)
```

When the wrapped type starts with `mut`, group it before adding `?`:

```ard
struct Widget {}
let maybe_ref: (mut Widget)? = Maybe::new()
```

Working with `Maybe` values:

```ard
let maybe_name: Str? = Maybe::new("Alice")

// Pattern matching
match maybe_name {
  name => "Hello, {name}!"
  _ => "Hello, stranger!"
}

// Checking presence
let has_value: Bool = maybe_name.is_some()
let is_empty: Bool = maybe_name.is_none()

// Providing defaults
let name: Str = maybe_name.or("Anonymous")
```

See [Maybe](/stdlib/maybe/) for the full API.

### Results

Fallible operations return a `Result`, written `T!E`. Where Go returns `(T, error)` and trusts the caller to check it, Ard makes the error part of the type and the compiler enforces handling.

```ard
fn divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("division by zero")
  } else {
    Result::ok(a / b)
  }
}
```

See [Error Handling](/guide/error-handling/) and [ard/result](/stdlib/result/) for details.

### Type Unions

Type unions allow a value to be one of several types:

```ard
type Printable = Str | Int
type Value = Int | Float64 | Str

let item: Printable = "Hello"
let data: Value = 42
```

Use match expressions to handle each type in a union:

```ard
use go:fmt

type Content = Str | Int | Bool

fn describe(value: Content) Str {
  match value {
    Str => "Text: {it}"
    Int => "Number: {it}"
    Bool => "Flag: {it}"
  }
}

let items: [Content] = ["hello", 42, true]
for item in items {
  fmt::Println(describe(item))
}
```

The `it` variable is automatically bound to the matched value.

## Type Inference

The compiler infers types from context, so annotations are usually optional:

```ard
let count = 42
let items = [1, 2, 3]
let scores = ["Alice": 95, "Bob": 87]
```

## Generic Syntax

Use a `$` prefix on a type to indicate a generic (unspecified) type:

```ard
fn identity(value: $T) $T {
  value
}
```

See [Generics](/advanced/generics/) for more.
