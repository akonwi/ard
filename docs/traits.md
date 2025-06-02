# Traits

The Rust definition of traits is:

> A trait defines the functionality a particular type has and can share with other types. We can use traits to define shared behavior in an abstract way. We can use trait bounds to specify that a generic type can be any type that has certain behavior.

> Note: Traits are similar to a feature often called interfaces in other languages, although with some differences.

Ard traits are heavily inspired by Rust.

## Defining a trait

Traits consists of the methods we can call on that type. Different types share the same behavior if we can call the same methods on all of those types. Trait definitions are a way to group method signatures together to define a set of behaviors necessary to accomplish some purpose.

For instance, the `io::print()` function expects a string and interpolation can include strings. Unfortunately that won't accept a custom type.
To overcome this, the `String` trait can be implemented to allow the custom type to conform to a string where necessary.

This is the `String` trait.

```ard
trait String {
  fn to_str() Str
}
```

A trait can have multiple methods in its body: the method signatures are listed one per line.

## Implementing a Trait on a Type

```ard
struct Person {
  name: Str
  age: Int
}

impl String for Person {
  fn to_str() Str {
    "{@.name} is {@age.to_str()}"
  }
}
```

## Using Traits
Traits can be used as function parameters to support functions on multiple types.

```ard
fn debug(thing: String) {
  io::print(thing.to_str())
}
let p = Person{name: "joe", age: 20}
debug(p)
```
