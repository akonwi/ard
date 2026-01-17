---
title: Optional Values with ard/maybe
description: Work with optional values using the ard/maybe module.
---

The `ard/maybe` module provides functions for working with optional values. The `Maybe` type (written as `T?`) represents a value that may or may not be present.

The maybe module provides:
- **Value creation** with `maybe::some()` and `maybe::none()`
- **Nullable types** for safe representation of optional values
- **Type safety** to prevent null pointer errors at compile time

```ard
use ard/maybe
use ard/io

fn main() {
  let maybe_name: Str? = maybe::some("Alice")
  
  match maybe_name {
    name => io::print("Hello, {name}"),
    _ => io::print("Hello, stranger")
  }
}
```

## API

### `fn some(val: $T) $T?`

Create a `Maybe` value containing the given value.

```ard
use ard/maybe

let value = maybe::some(42)
```

### `fn none() $T?`

Create an empty `Maybe` value. Type parameters must be explicitly provided.

```ard
use ard/maybe

let empty: Int? = maybe::none()
```

## Maybe Type Methods

All `Maybe` types have the following methods:

### `fn is_some() Bool`

Check if the Maybe contains a value.

```ard
use ard/maybe

let val: Int? = maybe::some(42)
if val.is_some() {
  // has a value
}
```

### `fn is_none() Bool`

Check if the Maybe is empty.

```ard
use ard/maybe

let val: Int? = maybe::none()
if val.is_none() {
  // is empty
}
```

### `fn or(default: $T) $T`

Get the value from the Maybe, or return a default if it's empty.

```ard
use ard/maybe

let val: Int? = maybe::none()
let result = val.or(0)  // 0
```

## Pattern Matching with Maybe

Use `match` expressions to safely handle optional values:

```ard
use ard/maybe
use ard/io

fn main() {
  let maybe_age: Int? = maybe::some(30)
  
  match maybe_age {
    age => io::print("Age: {age.to_str()}"),
    _ => io::print("Age unknown")
  }
}
```

When a `Maybe` value is matched:
- The first pattern captures the inner value if present
- The `_` pattern matches when the value is absent (none)

## Examples

### Check for Presence

```ard
use ard/maybe
use ard/io

fn main() {
  let email: Str? = maybe::none()
  
  if email.is_some() {
    io::print("Email: {email.or("")}")
  } else {
    io::print("No email provided")
  }
}
```

### Provide Defaults

```ard
use ard/maybe

fn main() {
  let theme: Str? = maybe::none()
  let selected_theme = theme.or("light")
  // selected_theme is "light"
}
```

### Process Optional Data

```ard
use ard/maybe

struct User {
  name: Str,
  bio: Str?
}

fn main() {
  let user = User {
    name: "Alice",
    bio: maybe::some("Software engineer")
  }
  
  match user.bio {
    description => {
      // has bio
    },
    _ => {
      // no bio
    }
  }
}
```

### Chain Operations with Maybe

```ard
use ard/maybe
use ard/io

fn get_user_name(user_id: Int) Str? {
  if user_id == 1 {
    maybe::some("Alice")
  } else {
    maybe::none()
  }
}

fn main() {
  let name = get_user_name(1)
  io::print(name.or("Unknown user"))
}
```

### Work with Lists of Optional Values

```ard
use ard/maybe
use ard/list

fn main() {
  let values: [Int?] = [
    maybe::some(1),
    maybe::none(),
    maybe::some(3)
  ]
  
  // Using list operations with optional values
}
```
