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

When a field's value comes from a variable with the same name, omit the colon and value:

```ard
let name = "Alice"
let age = 30
let email = "alice@example.com"

let person = Person{name, age, email}
```

This is equivalent to `Person{name: name, age: age, email: email}`.

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

`name` and `omit` may be combined. `skip` cannot be combined with either. JSON names must be unique within a struct. Names must also be representable by Go 1.27 JSON struct tags: they cannot be empty, equal `"-"`, or contain commas, backslashes, quotes, apostrophes, or backticks. Other UTF-8 names, including spaces and Unicode, are supported.

Attributes are currently supported only on Ard-owned struct fields. In addition to semantic `#json` metadata, fields may carry [opaque Go struct tags](/advanced/go-interop#go-struct-tags-on-ard-structs) for direct interop.

## Go Struct Tags

Use `#go:<key>("<value>")` when a reflection-based Go library requires a struct tag:

```ard
struct Config {
  #go:yaml("global_context,omitempty")
  global_context: Str,

  #go:validate("required")
  name: Str,
}
```

The value is passed to Go unchanged; Ard does not interpret library-specific options. Each key may appear once per field, and `#go:json` is reserved—use `#json` for JSON behavior. Different Go tags and `#json` may be combined on one field.

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

A `fn mut` method needs a `*mut Person` receiver. Calling it does not take the receiver's address implicitly, so hold the value through a pointer, or take one with `&mut`:

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

let bob = &mut Person{name: "Bob", age: 30}
bob.grow_older() // OK: bob is a *mut Person

mut alice = Person{name: "Alice", age: 30}
alice.age = 31             // OK: field write on a mut binding
// alice.grow_older()      // Error: requires a *mut Person receiver
(&mut alice).grow_older()  // OK
```

Non-mutating methods accept any receiver, including `*Person` and `*mut Person` pointers.

## Pointer-valued fields

A field typed as `*mut T` or `*T` stores a pointer:

```ard
struct Session {
  user: *mut Person,
}

mut first = Person{name: "Ada", age: 30}
mut second = Person{name: "Grace", age: 35}
mut session = Session{user: &mut first}

session.user.grow_older()      // writes first through the pointer
session.user = &mut second     // repoints the field; needs a writable session
```

Writing through a pointer field targets the pointee, so it works even when the containing value is bound with `let`. Replacing the field itself writes the containing value's storage. Copying `session.user` copies the pointer. Use `session.user.*` when a `Person` value is required.

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
