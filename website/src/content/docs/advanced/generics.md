---
title: Generics
description: Learn about generic programming in Ard using type parameters.
---

## Overview

Generics allow writing code that works with multiple types while maintaining type safety. Ard uses a simple syntax for generic types that doesn't require explicit declarations.

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
let floats = map<Int, Float>(ints, Float::from_int)
```

Type arguments correspond to the order of generics declared in the signature.

## Generic Structs

Structs can also hold generics:

```ard
struct Container<$T> {
  value: $T
}

let int_container = Container { value: 42 }
let str_container = Container { value: "hello" }
```
