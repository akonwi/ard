---
title: Modules
description: Learn about Ard's module system, imports, and code organization.
---

## Module Basics

Each Ard file is a module that can be either a runnable program or used by other modules. Imports are declared at the top of files using the `use` keyword.

```ard
use ard/io
use my_project/utils as helpers

fn main() {
  io::print("Hello from main module")
  helpers::calculate(42)
}
```

A module with a `main` function is a program and can be run with `ard run [path]`.

## Import Syntax

The basic import syntax uses absolute paths from the project root:

```ard
use path/to/module
use path/to/module as alias
```

By default, the imported module is available with the last segment of the path as the name. Use `as` to provide a custom name.

## Standard Library

The Ard standard library consists of modules under the `ard/*` path:

```ard
use ard/io          // Input/output functions
use ard/json        // JSON parsing and serialization
use ard/http        // HTTP client functionality
use ard/async       // Asynchronous programming
use ard/maybe       // Maybe type utilities
```

## Project Structure

Import paths are always absolute from the project root, determined by the presence of an `ard.toml` file:

```
my_calculator/
├── ard.toml          # Project configuration
├── main.ard          # Entry point
├── utils.ard         # Utility functions
└── math/
    └── operations.ard # Math operations
```

### Project Configuration

The `ard.toml` file defines the project:

```toml
name = "my_calculator"
```

If no `ard.toml` file is present, the project name defaults to the root directory name.

## Import Examples

With the above project structure:

```ard
// In main.ard
use my_calculator/utils
use my_calculator/math/operations

fn main() {
  let result = operations::add(5, 3)
  utils::log("Calculation complete")
}
```

```ard
// In utils.ard
use ard/io

fn log(message: Str) {
  io::print("[LOG] {message}")
}

fn format_number(num: Int) Str {
  "Number: {num.to_str()}"
}
```

```ard
// In math/operations.ard
fn add(a: Int, b: Int) Int {
  a + b
}

fn multiply(a: Int, b: Int) Int {
  a * b
}

fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("Division by zero")
    false => Result::ok(a / b)
  }
}
```

## Public and Private Declarations

### Public by Default

All declarations in a module are public and accessible from other modules by default:

```ard
// In utils.ard
fn helper_function() Str {  // Public
  "This can be called from other modules"
}

let global_config = "config_value"  // Public
```

### Private Declarations

Use the `private` keyword to make declarations module-local:

```ard
// In utils.ard
fn public_function() Str {
  private_helper()  // OK: same module
}

private fn private_helper() Str {  // Private
  "This cannot be called from other modules"
}

private let secret_key = "abc123"  // Private
```

## Struct Field Visibility

Struct fields are always public if the struct is public.

```ard
// In user.ard
struct User {
  id: Int          // Public
  username: Str    // Public
  email: Str       // Public
}

// Methods can be private
impl User {
  fn get_display_name() Str {  // Public
    format_name(@username)     // Calls private method
  }

  private fn format_name(name: Str) Str {  // Private
    "User: {name}"
  }
}
```
