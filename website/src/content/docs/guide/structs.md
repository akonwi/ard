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
struct Person {
  name: Str,
  age: Int,
  email: Str,
}

let person = Person{
  name: "Alice",
  age: 30,
  email: "alice@example.com",
}
```

## Accessing Fields

Use dot notation to access struct fields:

```ard
use go:fmt

struct Person {
  name: Str,
  age: Int,
}

let person = Person{name: "Alice", age: 30}

let name = person.name        // "Alice"
let age = person.age          // 30
fmt::Println("Hello, {person.name}!")
```

## Nullable Fields

Struct fields can be nullable using the `?` suffix. Nullable fields can be omitted when creating an instance, in which case they default to `none`:

```ard
struct Config {
  name: Str,
  timeout: Int?,
  retries: Int?,
}

// Omit nullable fields — they become none
let default_config = Config{name: "app"}

// Provide values directly — they are automatically wrapped
let custom = Config{name: "app", timeout: 30, retries: 3}

// You can still use Maybe::new() explicitly if you prefer
let explicit = Config{name: "app", timeout: Maybe::new(30)}
```

This is the same implicit wrapping behavior available for [nullable function parameters](/guide/functions#nullable-parameters).

## JSON Field Metadata

Ard structs work directly with Go's JSON APIs. By default, every field uses its original Ard name in JSON, including names converted when generating exported Go fields.

Use `#json` immediately before a field to customize its JSON representation:

```ard
struct User {
  #json(name: "displayName")
  display_name: Str,

  #json(omit: none)
  nickname: Str?,

  #json(skip: true)
  password_hash: Str,
}
```

- `name: "..."` changes the object-member name for marshaling and unmarshaling.
- `omit: none` is valid on nullable fields. It omits `none`, but retains present empty values such as `some("")`.
- `skip: true` excludes the field from marshaling and unmarshaling. It does not make the field optional when constructing an Ard value.

`name` and `omit` may be combined. `skip` cannot be combined with either. JSON names must be unique within a struct.

Attributes are currently supported only on struct fields, and `#json` is the only available attribute.

## Methods

Methods are like normal functions and are only available on instances of a struct.

Use `impl` blocks to define struct methods.

```ard
struct Rectangle {
  width: Float64,
  height: Float64
}

impl Rectangle {
  fn area() Float64 {
    self.width * self.height
  }

  fn perimeter() Float64 {
    2.0 * (self.width + self.height)
  }

  fn is_square() Bool {
    self.width == self.height
  }
}
```

### The `self` Receiver

Within methods, use `self` to reference the current instance's fields:

```ard
struct Person {
  name: Str,
  age: Int,
}

impl Person {
  fn get_intro() Str {
    "My name is {self.name} and I am {self.age} years old"
  }

  fn is_adult() Bool {
    self.age >= 18
  }
}
```

### Mutating methods

Because Ard requires explicit data mutation, methods that can change the struct must be marked as mutating, with the `mut` keyword after `fn`.

```ard
struct Person {
  name: Str,
  age: Int,
}

impl Person {
  fn mut grow_older() {
    self.age =+ 1
  }
}
```

This signature lets the compiler require an actual `mut Person` reference receiver. A writable ordinary binding is not enough: `mut bob = Person{...}` permits replacing `bob`, but it still stores an ordinary value.

```ard
struct Person {
  name: Str,
  age: Int,
}

impl Person {
  fn mut grow_older() {
    self.age =+ 1
  }
}

let bob = Person{name: "Bob", age: 30}
let bob_reference = mut bob
bob_reference.grow_older() // OK

mut alice = Person{name: "Alice", age: 30}
// alice.grow_older() // Error: alice stores an ordinary Person
alice = Person{name: "Alice", age: 31} // OK: replaces alice's slot
```

## Reference-valued fields

A field typed as `mut T` stores a reference handle. Initialization and rebinding require actual references:

```ard
struct Session {
  user: mut Person,
}

let first = Person{name: "Ada", age: 30}
let second = Person{name: "Grace", age: 35}
let session = mut Session{user: mut first}

session.user.grow_older()
session.user = mut second
```

Copying `session.user` copies its current reference handle. Rebinding the field changes only that field slot; previously copied references keep their original pointee. Use `session.user.@` when an ordinary `Person` field or value is required.

## Method Privacy

Methods can be made private with the `private` keyword:

```ard
struct User {
  username: Str,
}

impl User {
  private fn format_name(name: Str) Str {
    "User: {name}"
  }

  fn get_display_name() Str {
    self.format_name(self.username) // Calls private method
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
  title: Str,
  completed: Bool,
}

// Static constructor function
fn Todo::new(title: Str) Todo {
  Todo{title: title, completed: false}
}

let todo = Todo::new("Learn Ard")
```
