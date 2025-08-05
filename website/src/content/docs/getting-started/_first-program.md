---
title: Your First Program
description: Write and run your first Ard program.
---

## Hello, World!

Create a file named `hello.ard` with the following content:

```ard
use ard/io

fn main() {
  io::print("Hello, World!")
}
```

Run the program:

```bash
ard run hello.ard
```

This will output:
```
Hello, World!
```

## Program Structure

Every Ard program consists of:

1. **Import statements** (optional): Use `use` to import modules
2. **Function definitions**: Define functions with `fn`
3. **Main function**: The entry point `main()` function

## Basic Syntax Elements

### Imports
```ard
use ard/io          // Standard library module
use my_project/utils // Project module
```

### Functions
```ard
fn greet(name: Str) {
  io::print("Hello, {name}!")
}
```

### Variables
```ard
let name = "Ard"        // Immutable
mut count = 0           // Mutable
```

## A More Complete Example

```ard
use ard/io

fn fibonacci(n: Int) Int {
  match n {
    0 => 0
    1 => 1
    _ => fibonacci(n - 1) + fibonacci(n - 2)
  }
}

fn main() {
  let numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
  
  for num in numbers {
    let result = fibonacci(num)
    io::print("fibonacci({num.to_str()}) = {result.to_str()}")
  }
}
```

This demonstrates:
- Pattern matching with `match`
- String interpolation with `{}`
- Type conversion with `.to_str()`
- List iteration with `for`

## Running Programs

Ard provides several ways to execute code:

```bash
# Run directly
ard run program.ard

# Run with arguments (if your program accepts them)
ard run program.ard arg1 arg2

# Check syntax without running
ard check program.ard
```

## Project Structure

For larger programs, organize code into modules:

```
my_project/
├── ard.toml          # Project configuration
├── main.ard          # Entry point
├── utils.ard         # Utility functions
└── math/
    └── operations.ard # Math operations
```

The `ard.toml` file defines the project:

```toml
name = "my_project"
```

Import modules using absolute paths from the project root:

```ard
use my_project/utils
use my_project/math/operations
```

Now proceed to the [Language Guide](/guide/variables/) to learn about Ard's features in detail.