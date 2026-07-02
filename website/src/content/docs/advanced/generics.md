---
title: Generics
description: Learn about generic programming in Ard using type parameters.
---

## Overview

Generics allow writing code that works with multiple types while maintaining type safety. Ard uses a simple syntax where function generics are inferred from `$T` usage, and structs may explicitly declare receiver-level generics when needed.

## Generic Syntax

Generic types begin with `$` in function and struct declarations:

```ard
fn map(list: [$A], transform: fn($A) $B) [$B] {
  mut result: [$B] = []
  for item in list {
    result.push(transform(item))
  }
  result
}
```

In this example, `$A` and `$B` are generic type parameters. The function accepts a list of type `$A` and returns a list of type `$B`.

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

## Explicit Type Arguments

When type inference isn't sufficient, provide explicit type arguments:

```ard
let ints = [1, 2, 3]
let labels = map<Int, Str>(ints, fn(value: Int) Str { value.to_str() })
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
