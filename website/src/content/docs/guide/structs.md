---
title: Structs
description: Learn about defining and using structs, methods, and static functions in Ard.
---

Structs can be used for custom data types packaging multiple related values, like objects in most object-oriented languages.

## Defining Structs

```ard
struct Person {
  name: Str,
  age: Int,
  email: Str,
}
```

## Creating Struct Instances

```ard
let person = Person {
  name: "Alice",
  age: 30,
  email: "alice@example.com",
}
```

## Accessing Fields

Use dot notation to access struct fields:

```ard
let name = person.name        // "Alice"
let age = person.age          // 30
io::print("Hello, {person.name}!")
```

## Methods

Methods are like normal functions and are only available on instances of a struct.

Use `impl` blocks to define struct methods.

```ard
struct Rectangle {
  width: Float,
  height: Float
}

impl Rectangle {
  fn area() Float {
    @width * @height
  }

  fn perimeter() Float {
    2.0 * (@width + @height)
  }

  fn is_square() Bool {
    @width == @height
  }
}
```

### The `@` Prefix

Within methods, use the `@` prefix to reference the current instance's fields:

```ard
impl Person {
  fn get_intro() Str {
    "My name is {@name} and I am {@age} years old"
  }

  fn is_adult() Bool {
    @age >= 18
  }
}
```

### Mutating methods

Because Ard requires explicit data mutation, methods that can change the struct must be marked as mutating, with the `mut` keyword after `fn`.

```ard
impl Person {
  fn mut grow_older() {
    @age =+ 1
  }
}
```

This method signature signals helps the compiler enforce the immutability constraints.
Mutating methods can only be called on mutable instances.

```ard
let alice = Person {...}
alice.grow_older() // Cannot mutate immutable 'alice'

mut alice = Person {...}
alice.grow_older() // Ok
```

## Method Privacy

Methods can be made private with the `private` keyword:

```ard
impl User {
  fn get_display_name() Str {
    format_name(@username)  // Calls private method
  }

  private fn format_name(name: Str) Str {
    "User: {name}"
  }
}
```

Private methods can only be called from within the same module. <a href="/guide/modules">Read more about modules.</a>

## Static Functions

Static functions are functions declared in a struct's namespace.
These functions are distinct from methods because they do not operate on an instance.
They are primarily a way to organize code and signal related functionality.

The most common use of static functions is for constructors or factory helpers.

```ard
struct Todo {
  title: Str
  completed: Bool
}

// Static constructor function
fn Todo::new(title: Str) Todo {
  Todo { title: title, completed: false }
}

let todo = Todo::new("Learn Ard")
```
