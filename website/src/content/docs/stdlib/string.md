---
title: String Conversion with ard/string
description: Define custom string conversion for types using the ToString trait.
---

The `ard/string` module defines the `ToString` trait, which allows custom types to be converted to strings. This trait is fundamental to Ard's type system and is used throughout the standard library.

:::note
The `ard/string` module is a prelude module. It is automatically imported and aliased as `Str` in all programs, allowing the `ToString` trait to be accessed with the `Str::ToString` namespace.
:::

The string module provides:
- **ToString trait** for implementing custom string conversion
- **Integration** with `io::print` and string interpolation
- **Built-in string helper methods** for querying and transforming `Str` values
- **UTF-8 byte and Unicode scalar views** through `[Byte]` and `[Rune]`

```ard
use ard/string as Str

struct Person {
  name: Str,
  age: Int
}

impl Str::ToString for Person {
  fn to_str() Str {
    "{self.name} ({self.age})"
  }
}

fn main() {
  let person = Person { name: "Alice", age: 30 }
  io::print(person)  // Uses to_str() automatically
}
```

## API

### Built-in `Str` methods

Strings include common helper methods directly on `Str` values:

```ard
let path = "posts/hello.md"

path.starts_with("posts/") // true
path.ends_with(".md")      // true
path.contains("hello")     // true
path.at(0)                  // some('p')
path.bytes()                // UTF-8 bytes as [Byte]
path.runes()                // Unicode scalar values as [Rune]
path.is_empty()             // false
path.size()                 // 14 (UTF-8 byte length)
path.replace(".md", ".html")
path.replace_all("/", "-")
Str::split(path, "/")
path.trim()
```

#### `at(index: Int) Rune?`

Returns the Unicode scalar value at the zero-based rune `index`, or `none` when the index is out of bounds. Call `to_str()` when you need a one-rune string.

```ard
"hello".at(0).expect("missing").to_str() // "h"
"hé".at(1).expect("missing").to_str()    // "é"
"hello".at(5).is_none()                   // true
```

#### `bytes() [Byte]`, `runes() [Rune]`, and `size() Int`

`bytes()` returns UTF-8 bytes, `runes()` returns Unicode scalar values, and `size()` returns UTF-8 byte length.

```ard
"hé".bytes().size() // 3
"hé".runes().size() // 2
"hé".size()         // 3

for ch in "a/b" {
  if ch == '/' {
    // ch is a Rune
  }
}
```

#### `Str::split(input: Str, delimiter: Str) [Str]`

Splits a string from the `ard/string` module. An empty delimiter returns one-rune strings.

```ard
Str::split("a,b", ",") // ["a", "b"]
Str::split("hé", "")   // ["h", "é"]
```

#### `starts_with(prefix: Str) Bool`

Returns `true` when the string begins with `prefix`. An empty prefix always matches.

```ard
"hello".starts_with("he") // true
"hello".starts_with("lo") // false
```

#### `ends_with(suffix: Str) Bool`

Returns `true` when the string ends with `suffix`. An empty suffix always matches.

```ard
"hello".ends_with("lo") // true
"hello".ends_with("he") // false
```

### `trait ToString`

A trait that types can implement to define how they should be converted to strings.

```ard
use ard/string as Str

trait ToString {
  fn to_str() Str
}
```

## Implementing ToString

To implement `ToString` for a custom type, use the `impl` syntax:

```ard
use ard/string as Str

struct Point {
  x: Int,
  y: Int
}

impl Str::ToString for Point {
  fn to_str() Str {
    "({self.x}, {self.y})"
  }
}
```

## Built-in Implementations

The following built-in types have `to_str()` methods:
- **Str**: Returns itself
- **Int**: Converts to decimal string representation
- **Float64**: Converts to decimal string representation
- **Bool**: Returns "true" or "false"

## Examples

### Implement ToString for Enum

```ard
use ard/string as Str
use ard/io

enum Color {
  red,
  green,
  blue
}

impl Str::ToString for Color {
  fn to_str() Str {
    match self {
      Color::red => "Red",
      Color::green => "Green",
      Color::blue => "Blue"
    }
  }
}

fn main() {
  let color = Color::red
  io::print(color)  // "Red"
}
```

### Implement ToString for Struct

```ard
use ard/string as Str
use ard/io

struct Rectangle {
  width: Int,
  height: Int
}

impl Str::ToString for Rectangle {
  fn to_str() Str {
    "Rectangle {width: {self.width}, height: {self.height}}"
  }
}

fn main() {
  let rect = Rectangle { width: 10, height: 20 }
  io::print(rect)  // "Rectangle {width: 10, height: 20}"
}
```

### Use in String Interpolation

```ard
use ard/string as Str
use ard/io

struct Date {
  year: Int,
  month: Int,
  day: Int
}

impl Str::ToString for Date {
  fn to_str() Str {
    "{self.year}-{self.month}-{self.day}"
  }
}

fn main() {
  let today = Date { year: 2024, month: 1, day: 15 }
  io::print("Today is {today}")
}
```

### Use with Functions

```ard
use ard/string as Str

fn print_value(val: Str::ToString) {
  io::print(val)
}

fn main() {
  print_value(42)
  print_value("hello")
  print_value(true)
}
```
