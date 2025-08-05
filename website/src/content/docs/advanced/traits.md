---
title: Traits
description: Learn about defining and implementing traits for shared behavior in Ard.
---

## What are Traits?

Traits define behaviors that can be implemented by custom types. They are similar to interfaces in other languages but with some key differences. The Rust definition applies well to Ard:

> A trait defines the functionality a particular type has and can share with other types. We can use traits to define shared behavior in an abstract way.

## Defining Traits

Traits consist of method signatures that an implementing types must provide:

Here is a trait that is part of the standard library in the scope of the `Str` type.

```ard
trait ToString {
  fn to_str() Str
}
```

> 💡 `Str::ToString` is the trait that standard `io::print` function accepts and the built-in primitives implement it.

A trait can have multiple methods:

```ard
trait Drawable {
  fn draw()
  fn get_bounds() Rectangle
  fn is_visible() Bool
}
```

## Implementing Traits

Use `impl TraitName for TypeName` to implement a trait for a specific type:

```ard
struct Person {
  name: Str,
  age: Int,
}

impl Str::ToString for Person {
  fn to_str() Str {
    "{@name} is {@age} years old"
  }
}
```

## Using Traits

### As Function Parameters

Traits can be used as function parameter types to accept any type that implements the trait:

```ard
fn debug(thing: Str::ToString) {
  io::print(thing.to_str())
  thing.name // Error
}

let person = Person { name: "Alice", age: 30 }
debug(person)
```
