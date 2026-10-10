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

### Mutating trait methods

Mark a trait method with `fn mut` when implementations may mutate their
receiver:

```ard
trait Counter {
  fn mut set(value: Int)
  fn value() Int
}
```

Calling `set` requires mutable receiver access:

```ard
fn update(counter: mut Counter) {
  counter.set(2)
}

fn inspect(counter: Counter) Int {
  counter.value()
}
```

`mut Counter` is a trait value that may call `fn mut` methods. It is not a
pointer: create one by passing a `&mut T` whose `T` implements the trait. A
`mut Counter` converts to an ordinary `Counter` without copying the
underlying value:

```ard
struct Tally { count: Int }

impl Counter for Tally {
  fn mut set(value: Int) { self.count = value }
  fn value() Int { self.count }
}

let tally = &mut Tally{count: 0}
update(tally)              // &mut Tally widens to mut Counter
let total = inspect(tally) // 2
```

`mut Trait` is the only type that uses `mut`. Pointer types such as `&Counter`
are rejected; point to the concrete type instead. To keep an independent copy,
dereference the concrete pointer before widening: `let copy: Counter = tally.*`.

A mutating implementation cannot satisfy a trait method that omits `mut`.
A non-mutating implementation may satisfy a mutating method because it requires
less receiver capability than the contract permits.

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
