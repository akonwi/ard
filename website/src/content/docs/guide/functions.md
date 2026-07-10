---
title: Functions
description: Learn about function definition, parameters, return values, and advanced features in Ard.
---

## Function Definition

Functions are defined using the `fn` keyword:

```ard
fn greet(name: Str) Str {
  "Hello, {name}!"
}
```

## Parameters and Return Types

Function parameters require type annotations. Return types are specified after the parameter list.
Without an explicit return type, Ard will treat the function as non-returning.

```ard
use go:fmt

fn add(a: Int, b: Int) Int {
  a + b
}

// No return type specified - this function will not return a value.
// Equivalent to declaring `Void` as the return type
fn print_message(msg: Str) {
  fmt::Println(msg)
}
```

### Mutating parameters

In order for a function to apply side-effects or mutations to parameters, the parameter type must be marked as mutable in the signature.

```ard
use go:fmt

fn add_ten(value: mut Int) {
  value =+ 10
}

mut count = 0
add_ten(count)
fmt::Println(count) // 10
```

## Return Values

There is no `return` keyword in Ard. The last expression in a function is automatically returned:

```ard
fn multiply(x: Int, y: Int) Int {
  x * y
}

fn get_status(code: Int) Str {
  match code {
    200 => "OK"
    404 => "Not Found"
    500 => "Server Error"
    _ => "Unknown"
  }
}
```

## Nullable Parameters

Function parameters can be marked as nullable using the `?` modifier, allowing callers to omit them:

```ard
fn greet(name: Str, greeting: Str?) Str {
  let msg = greeting.or("Hello")
  "{msg}, {name}!"
}

// Providing a value for the nullable parameter
greet("Alice", "Hi")

// Omitting the nullable parameter (greeting becomes None)
greet("Bob")
```

When a non-nullable value is provided to a nullable parameter, it's automatically wrapped in `Maybe::new()`:

```ard
struct Options {
  verbose: Bool,
}

fn process(data: Str, options: Options?) {
  let opts = options.or(Options{verbose: false})
  // Process with options
}

// Automatically wraps the provided value in Maybe
process("data", Options{verbose: true})

// Omits the parameter (becomes none)
process("data")
```

### Omitting Nullable Parameters

You can omit any trailing nullable parameters in a function call. They will be treated as `None`:

```ard
fn configure(name: Str, timeout: Int?, retries: Int?, debug: Bool?) {
  // All nullable parameters are optional
}

// You can provide all, some, or none of the nullable parameters
configure("service", 30, 3, true)    // All provided
configure("service", 30, 3)          // debug omitted
configure("service", 30)             // retries and debug omitted
configure("service")                 // All nullable params omitted
```

## Labelled Arguments

Functions can be called with labelled arguments, allowing parameters to be specified in any order:

```ard
struct User {
  name: Str,
  age: Int,
  email: Str,
}

fn create_user(name: Str, age: Int, email: Str) User {
  User{name: name, age: age, email: email}
}

// Positional arguments (order matters)
create_user("Alice", 25, "alice@example.com")

// Named arguments (order doesn't matter)
create_user(age: 30, email: "bob@example.com", name: "Bob")
```

Positional and named arguments can be mixed, but positional arguments must come first:

```ard
// Allowed: positional, then named
create_user("Charlie", age: 35, email: "charlie@example.com")

// NOT allowed: positional after named
create_user(name: "Charlie", 35, "charlie@example.com")
```

## First-Class Functions

Functions are first-class values and can be used as arguments:

```ard
fn map(list: [Int], transform: fn(Int) Int) [Int] {
  mut mapped: [Int] = []
  for item in list {
    mapped.push(transform(item))
  }
  mapped
}

fn double(x: Int) Int {
  x * 2
}

// Pass function as argument
let numbers = [1, 2, 3, 4]
let doubled = map(numbers, double)
```

## Anonymous Functions

Functions can be defined inline without names:

```ard
fn map(list: [Int], transform: fn(Int) Int) [Int] {
  mut mapped: [Int] = []
  for item in list {
    mapped.push(transform(item))
  }
  mapped
}

let squared = map([1, 2, 3], fn(x: Int) Int { x * x })
```

## Function Signatures

When referring to function types, use the `fn` syntax and just omit the body:

```ard
use go:fmt

fn add(a: Int, b: Int) Int { a + b }
fn shout(msg: Str) { fmt::Println(msg) }
fn get_random_number() Int { 4 }

let operation: fn(Int, Int) Int = add
let printer: fn(Str) = shout
let generator: fn() Int = get_random_number
```

Use `?` after the function type for nullable function values. If the function type has an explicit return type, wrap the whole type in parentheses so the `?` applies to the function instead of the return type:

```ard
fn lookup_name(id: Int) Str? {
  Maybe::new<Str>()
}

let optional_printer: fn(Str)? = Maybe::new()          // nullable fn(Str) Void
let optional_mapper: (fn(Int) Str)? = Maybe::new()     // nullable fn(Int) Str
let maybe_name: fn(Int) Str? = lookup_name             // non-null function returning Str?
```

`fn(Int) Void?` is rejected because it is ambiguous and usually means an optional callback. Use `fn(Int)?` or `(fn(Int) Void)?` instead.
