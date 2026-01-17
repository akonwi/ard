---
title: Input/Output with ard/io
description: Print to console and read input using Ard's I/O module.
---

The `ard/io` module provides basic input and output operations for console interaction.

The io module provides:
- **Console output** with the `print` function
- **Console input** with the `read_line` function
- **Automatic string conversion** via the ToString trait

```ard
use ard/io

fn main() {
  io::print("What's your name?")
  let name = io::read_line().expect("Failed to read")
  io::print("Hello, {name}!")
}
```

## API

### `fn print(value: Str::ToString)`

Print a value to the console. The value must implement the `Str::ToString` trait, which all built-in primitive types implement.

```ard
use ard/io

io::print("Hello, world!")
io::print(42)
io::print(true)
```

### `fn read_line() Str!Str`

Read a line from standard input. Returns a result containing the input string or an error if reading fails.

```ard
use ard/io

let input = io::read_line().expect("Failed to read line")
```

## Examples

### Basic Print and Read

```ard
use ard/io

fn main() {
  io::print("Hello, world!")
  
  io::print("Enter your name: ")
  let name = io::read_line().expect("Failed to read name")
  
  io::print("Hello, {name}!")
}
```

### Print Multiple Values

```ard
use ard/io

fn main() {
  io::print("Number: 42")
  io::print("Boolean: true")
  io::print("Float: 3.14")
}
```

### Read and Process Input

```ard
use ard/io
use ard/int

fn main() {
  io::print("Enter a number: ")
  let input = io::read_line().expect("Failed to read")
  
  match Int::from_str(input) {
    num => io::print("You entered: {num.to_str()}"),
    _ => io::print("Invalid input")
  }
}
```

### Display Struct

```ard
use ard/io

struct Person {
  name: Str,
  age: Int
}

fn main() {
  let person = Person { name: "Alice", age: 30 }
  io::print("Name: {person.name}")
  io::print("Age: {person.age}")
}
```
