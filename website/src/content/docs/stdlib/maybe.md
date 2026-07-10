---
title: Optional Values with Maybe
description: Work with optional values using the built-in Maybe type.
---

`Maybe` is a built-in optional type. It represents a value that may or may not be present and can be written either as the formal type `Maybe<T>` or the shorthand `T?`.

Maybe provides:
- **Value creation** with `Maybe::new(value)` and `Maybe::new<T>()`
- **Nullable types** for safe representation of optional values
- **Mutation helpers** (`set`/`clear`) for mutable optional slots
- **Type safety** to prevent null pointer errors at compile time

```ard
use go:fmt

fn main() {
  let maybe_name: Str? = Maybe::new("Alice")
  
  match maybe_name {
    name => fmt::Println("Hello, {name}"),
    _ => fmt::Println("Hello, stranger")
  }
}
```

## API

### `Maybe::new(value: $T?) Maybe<$T>`

Create a `Maybe` value. Pass a value to create a present value, or omit the nullable argument to create an empty value.

```ard
let value: Maybe<Int> = Maybe::new(42)
let shorthand: Int? = Maybe::new(42)
```

When the argument is omitted, the type parameter may be inferred from context, or provided explicitly when there is no context.

```ard
let empty: Int? = Maybe::new()
let explicit = Maybe::new<Int>()
```

## Maybe Type Methods

All `Maybe` types have the following methods:

### `fn is_some() Bool`

Check if the Maybe contains a value.

```ard

let val: Int? = Maybe::new(42)
if val.is_some() {
  // has a value
}
```

### `fn is_none() Bool`

Check if the Maybe is empty.

```ard

let val: Int? = Maybe::new()
if val.is_none() {
  // is empty
}
```

### `fn or(default: $T) $T`

Get the value from the Maybe, or return a default if it's empty.

```ard

let val: Int? = Maybe::new()
let result = val.or(0)  // 0
```

### `fn expect(message: Str) $T`

Get the value from the Maybe, or panic with a message if it's empty.

```ard

let val: Int? = Maybe::new(42)
let result = val.expect("expected a value")  // 42
```

### `fn map(with: fn($T) $U) $U?`

Transform a present value with a function that **returns a plain value**. The result is automatically wrapped as present. If the Maybe is empty, the callback is not called and the empty value passes through unchanged.

Use `map` when the transformation always produces a value.

```ard

let num: Int? = Maybe::new(21)
let doubled = num.map(fn(v) { v * 2 })
let value = doubled.or(0) // 42

// empty values pass through untouched
let empty: Int? = Maybe::new()
empty.map(fn(v) { v * 2 }).is_none() // true
```

You can also provide explicit type arguments when you want to guide inference:

```ard
let as_text = num.map<Str>(fn(v) { "{v}" })
```

### `fn and_then(with: fn($T) $U?) $U?`

Chain operations that **return a Maybe themselves** (also known as `flat_map` in other languages). Unlike `map`, the callback is responsible for returning `Maybe::new(value)` or `Maybe::new<T>()`. This lets the callback itself decide whether a value is present.

Use `and_then` when the next step might not produce a value.

```ard

fn even_only(num: Int) Int? {
  match num % 2 == 0 {
    true => Maybe::new(num),
    false => Maybe::new(),
  }
}

let result = Maybe::new(20).and_then(even_only)
result.is_some() // true

// The callback can return none, unlike map:
let odd = Maybe::new(21).and_then(even_only)
odd.is_none() // true
```

### `fn set(value: $T)`

Mutate a `Maybe<T>` slot to contain `value`. The receiver must be mutable.

```ard
mut current = Maybe::new<Int>()
current.set(42)
current.expect("set") // 42
```

### `fn clear()`

Mutate a `Maybe<T>` slot back to `none`. The receiver must be mutable.

```ard
mut current = Maybe::new("ready")
current.clear()
current.is_none() // true
```

## Pattern Matching with Maybe

Use `match` expressions to safely handle optional values:

```ard
use go:fmt

fn main() {
  let maybe_age: Int? = Maybe::new(30)
  
  match maybe_age {
    age => fmt::Println("Age: {age.to_str()}"),
    _ => fmt::Println("Age unknown")
  }
}
```

When a `Maybe` value is matched:
- The first pattern captures the inner value if present
- The `_` pattern matches when the value is absent (none)

## Examples

### Check for Presence

```ard
use go:fmt

fn main() {
  let email: Str? = Maybe::new()
  
  if email.is_some() {
    let address = email.or("unknown")
    fmt::Println("Email: {address}")
  } else {
    fmt::Println("No email provided")
  }
}
```

### Provide Defaults

```ard

fn main() {
  let theme: Str? = Maybe::new()
  let selected_theme = theme.or("light")
  // selected_theme is "light"
}
```

### Process Optional Data

Nullable struct fields accept unwrapped values directly — they are automatically wrapped in `Maybe::new()`:

```ard
struct User {
  name: Str,
  bio: Str?,
}

fn main() {
  // bio is automatically wrapped in Maybe::new()
  let user = User{
    name: "Alice",
    bio: "Software engineer",
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
use go:fmt

fn get_user_name(user_id: Int) Str? {
  if user_id == 1 {
    Maybe::new("Alice")
  } else {
    Maybe::new()
  }
}

fn main() {
  let name = get_user_name(1)
  fmt::Println(name.or("Unknown user"))
}
```

### Work with Lists of Optional Values

```ard
use ard/list

fn main() {
  let values: [Int?] = [
    Maybe::new(1),
    Maybe::new(),
    Maybe::new(3)
  ]
  
  // Using list operations with optional values
}
```
