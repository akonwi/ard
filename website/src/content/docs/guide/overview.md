---
title: Overview
description: The basics of Ard.
---

Ard is a statically-typed language that compiles to Go, has interoperablity with the Go ecosystem, while providing stricter and more explicit semantics.

## What Ard adds

### Rust-like error API

Just like Go, there are no exceptions. Fallible operations return a `Result` (written `T!E`), and the compiler requires callers to handle or propagate errors with `try`, `match`, or explicit methods.

```ard
fn divide(a: Int, b: Int) Int!Str {
  if b == 0 {
    Result::err("division by zero")
  } else {
    Result::ok(a / b)
  }
}
```

### No nil

Ard has no null or nil value. Optional values are explicit with `Maybe` (written `T?`), and the compiler forces absent cases to be handled before the value can be used.

```ard
let found: Int? = list::find(numbers, fn(n: Int) Bool { n > 10 })
let value = found.or(0)
```

### Immutability by default

Bindings, parameters, and struct fields are immutable unless marked `mut`. Mutation is visible in the source wherever it can happen.

```ard
let name = "Ada"   // immutable
mut count = 0      // mutable
count =+ 1
```

### One obvious way

Ard keeps its surface small: expression-based functions with no `return` keyword, a single `match` construct for branching on values and types, and left-to-right type syntax. There is usually one clear way to write a given thing.

## The Go relationship

Because Ard compiles to Go, the boundary between the two is intentionally thin:

- **Direct imports.** Go packages are imported with `use go:` and called directly — no bindings or wrapper layer required for APIs that map cleanly to Ard values.

  ```ard
  use go:fmt

  fn main() {
    fmt::Println("hello from Go")
  }
  ```

- **Boundary adaptation.** Idiomatic Go shapes are adapted at the call boundary: `(T, error)` returns become `T!Str`, comma-ok pairs become `T?`, and Go's `any` becomes Ard's opaque `Any`.

- **Concurrency is Go's.** `async::start` runs goroutines, and built-in channels (`Chan<T>`) lower to native Go channels, including `select`.

Ard semantics always come first: where Go and Ard disagree — nil, exceptions via panic, implicit zero values — Ard keeps its own rules and makes the Go behavior an explicit interop concern (see [Direct Go Interop](/advanced/go-interop/)).

## Where to go next

The rest of this guide walks through the language from the ground up:

1. [Types](/guide/types/) — primitives, collections, and type unions
2. [Variables](/guide/variables/) — `let`, `mut`, and mutability rules
3. [Functions](/guide/functions/) — parameters, named arguments, and closures
4. [Control Flow](/guide/control-flow/) — `if`, loops, and `match`
5. [Error Handling](/guide/error-handling/) — `Result`, `Maybe`, and `try`
