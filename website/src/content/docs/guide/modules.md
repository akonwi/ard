---
title: Modules
description: Learn about Ard's module system, imports, and code organization.
---

Each Ard file is a module. A file with a `main` function can be run as a program; other files can be imported by path.

```ard
use go:fmt
use my_calculator/math/operations as ops

fn main() {
  let result = ops::add(5, 3)
  fmt::Println("Result: {result}")
}
```

Run a program with:

```sh
ard run main.ard
```

## Import Syntax

Use `use` at the top of a file:

```ard
use path/to/module
use path/to/module as alias
```

By default, an imported Ard module is referenced by the last segment of its path.

Go packages use the same `use` syntax with a `go:` prefix:

```ard
use go:fmt
use go:strings
use go:net/http as gohttp
```

The imported Go package is available as a namespace, not as an Ard module.

## Standard Library Imports

Ard's standard library modules start with the `ard/` prefix:

```ard
use ard/list        // List helpers
use ard/map         // Map helpers
use ard/testing     // Test assertions
use ard/unsafe      // Interop escape hatches
```

## Project Structure

Project import paths are absolute from the package root. The package root is determined by the nearest `ard.toml`; if no manifest exists, Ard uses the root directory name as the package name.

```
my_calculator/
├── ard.toml
├── main.ard
├── utils.ard
└── math/
    └── operations.ard
```

A minimal `ard.toml` looks like this:

```toml
name = "my_calculator"
ard = ">= 0.0.0"
```

With that structure, `main.ard` can import sibling modules by package path:

```ard
use my_calculator/utils
use my_calculator/math/operations

fn main() {
  let result = operations::add(5, 3)
  utils::log("Calculation complete: {result}")
}
```

```ard
// utils.ard
use go:fmt

fn log(message: Str) {
  fmt::Println("[LOG] {message}")
}
```

```ard
// math/operations.ard
fn add(a: Int, b: Int) Int {
  a + b
}

fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero")
    false => Result::ok(a / b)
  }
}
```

A dependency can expose a root module whose filename matches its manifest name. If a dependency named `decode` contains `decode.ard`, consumers can load that root module without repeating its name:

```ard
use decode

let value = decode::run(input, decoder)
```

Nested dependency modules still use their full path, such as `use decode/path`. For dependency declarations, aliases, root modules, and lockfile behavior, see the [Dependencies](/guide/dependencies/) guide.

## Public and Private Declarations

Functions, structs, enums, traits, and immutable top-level variables are public by default. Use `private` to keep a declaration module-local.

```ard
// utils.ard
fn public_name() Str {
  private_name()
}

private fn private_name() Str {
  "internal"
}

struct Config {
  name: Str,
}

private struct InternalConfig {
  secret: Str,
}
```

Mutable top-level variables are private by default because they represent shared mutable module state.

```ard
// constants.ard
let API_URL = "https://api.example.com"  // public
let MAX_RETRIES = 3                      // public

mut internal_counter = 0                 // private
mut debug_mode = false                   // private
```

From another module, only public declarations are accessible:

```ard
use my_calculator/constants

fn main() {
  let url = constants::API_URL
  let max = constants::MAX_RETRIES

  // Not accessible from outside constants.ard:
  // let counter = constants::internal_counter
}
```

## Struct Fields and Methods

Fields of a public Ard struct are public. Methods are public by default and can be marked `private`.

```ard
struct User {
  id: Int,
  username: Str,
  email: Str,
}

impl User {
  private fn format_name(name: Str) Str {
    "User: {name}"
  }

  fn display_name() Str {
    self.format_name(self.username)
  }
}
```
