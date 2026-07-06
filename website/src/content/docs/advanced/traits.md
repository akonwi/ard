---
title: Traits
description: Learn about defining and implementing traits for shared behavior in Ard.
---

## What are Traits?

Traits define behaviors that can be implemented by custom types. They are similar to interfaces in other languages but with some key differences. The Rust definition applies well to Ard:

> A trait defines the functionality a particular type has and can share with other types. We can use traits to define shared behavior in an abstract way.

## Defining Traits

Traits consist of method signatures that implementing types must provide:

```ard
trait Describable {
  fn describe() Str
}
```

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
trait Describable {
  fn describe() Str
}

struct Person {
  name: Str,
  age: Int,
}

impl Describable for Person {
  fn describe() Str {
    "{self.name} is {self.age} years old"
  }
}
```

## Using Traits

### As Function Parameters

Traits can be used as function parameter types to accept any type that implements the trait:

```ard
use go:fmt

fn debug(thing: Describable) {
  fmt::Println(thing.describe())
}

let person = Person{name: "Alice", age: 30}
debug(person)
```

Inside `debug`, only the trait's methods are available. Accessing `thing.name` would be a compile-time error because `Describable` says nothing about a `name` field.
