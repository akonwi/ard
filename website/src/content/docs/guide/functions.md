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
fn add(a: Int, b: Int) Int {
  a + b
}

// No return type specified - this function will not return a value
fn print_message(msg: Str) {
  io::print(msg)
}
```

### Mutating parameters

In order for a function to apply side-effects or mutations to parameters, the parameter must be marked as mutable in the signature.

```ard
fn add_ten(mut value: Int) {
  value += 10
}

mut count = 0
add_ten(count)
io::print(count) // 10
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

## Labelled Arguments

Functions can be called with labelled arguments, allowing parameters to be specified in any order:

```ard
fn create_user(name: Str, age: Int, email: Str) User {
  User { name: name, age: age, email: email }
}

// Positional arguments (order matters)
create_user("Alice", 25, "alice@example.com")

// Named arguments (order doesn't matter)
create_user(age: 30, email: "bob@example.com", name: "Bob")
```

**Important**: When using named arguments, all arguments must be named. Mixing positional and named arguments is not supported.

```ard
// This is NOT allowed:
create_user("Charlie", age: 35, email: "charlie@example.com")
```

## First-Class Functions

Functions are first-class values and can be used as arguments:

```ard
fn map(list: [Int], transform: fn(Int) Int) [Int] {
  let mapped: [Int] = []
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
let squared = map([1, 2, 3], fn(x: Int) Int { x * x })
```

## Function Signatures

When referring to function types, use the `fn` syntax and just omit the body:

```ard
let operation: fn(Int, Int) Int = add
let printer: fn(Str) = io::print
let generator: fn() Int = get_random_number
```
