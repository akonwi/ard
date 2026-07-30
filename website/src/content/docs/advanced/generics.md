---
title: Generics
description: Learn about generic programming in Ard using type parameters.
---

## Overview

Generics allow writing code that works with multiple types while maintaining type safety. Ard uses a simple syntax where function generics are inferred from `$T` usage, and structs may explicitly declare receiver-level generics when needed.

## Generic Syntax

Generic types begin with `$` in function and struct declarations:

```ard
fn apply(value: $A, transform: fn($A) $B) $B {
  transform(value)
}
```

In this example, `$A` and `$B` are generic type parameters. The function accepts a value of type `$A` and returns the `$B` produced by the callback.

## Type Inference

The compiler attempts to infer generic types from usage:

```ard
fn identity(value: $T) $T {
  value
}

let number = identity(42)        // $T inferred as Int
let text = identity("hello")     // $T inferred as Str
let flag = identity(true)        // $T inferred as Bool
```

Reference identity is preserved during generic inference. If `$T` is inferred from a `mut User`, the result is also `mut User`:

```ard
struct User { name: Str }

let user = User{name: "Ada"}
let reference = mut user
let alias = identity(reference) // $T is mut User
alias.name = "Grace"
```

A generic destination explicitly fixed to ordinary `User` does not silently copy the referent. Use `deref` to select a shallow value:

```ard
let snapshot = identity<User>(deref reference)
```

The same rule applies when references appear inside generic lists, maps, `Maybe`, `Result`, channels, callbacks, and struct fields: the reference type remains part of the generic shape.

## Explicit Type Arguments

When type inference isn't sufficient, provide explicit type arguments:

```ard
let label = apply<Int, Str>(42, fn(value: Int) Str { value.to_str() })
```

Type arguments correspond to the order of generics introduced in the signature.

## Generic Structs

Structs can also hold generics. If a generic type appears in a field, that field introduces the struct's generic parameter:

```ard
struct Container {
  value: $T,
}

let int_container = Container{value: 42}
let str_container = Container{value: "hello"}
```

Generic parameters are introduced by fields and function signatures. Structs may also declare receiver-level generic parameters explicitly when methods need a generic that does not immediately appear in fields:

```ard
struct State<$T> {
  handle: StateHandle,
}

impl State {
  fn value() $T {
    panic("not implemented")
  }
}
```

Methods may use the generic parameters introduced by their receiver type, but they cannot introduce independent method-only generic parameters.

When referencing a generic struct as a type, provide concrete type arguments:

```ard
fn get_value(container: Container<Int>) Int {
  container.value
}
```
