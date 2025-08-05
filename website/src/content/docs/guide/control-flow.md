---
title: Control Flow
description: Learn about conditional statements, loops, and pattern matching in Ard.
---

## Conditional Statements

### If-Else

```ard
if temperature > 30 {
  io::print("It's hot!")
} else if temperature < 10 {
  io::print("It's cold!")
} else {
  io::print("Nice weather!")
}
```

Conditions must be boolean expressions. There are no implicit truthy/falsy coercions.

## Loops

### For Loops

#### Iterating Over Collections

```ard
let fruits = ["apple", "banana", "cherry"]
for fruit, index in fruits {
  io::print("{index}: {fruit}")
}
```

The index cursor can be omitted in list loops

```ard
let fruits = ["apple", "banana", "cherry"]
for fruit in fruits {
  io::print(fruit)
}
```

#### Iterating Over Maps

```ard
let scores: [Str:Int] = ["Alice": 95, "Bob": 87, "Carol": 92]
for name, score in scores {
  io::print("{name} scored {score.to_str()}")
}
```

#### Numeric Ranges

```ard
// Inclusive range
for i in 1..10 {
  io::print(i)
}

// With step (if supported)
for i in 0..100 step 10 {
  io::print(i)  // Prints 0, 10, 20, ..., 100
}
```

#### C-Style For Loop

```ard
for mut i = 0; i <= 5; i =+ 1 {
  io::print("Count: {i}")
}
```

### While Loops

```ard
mut count = 0
while count < 10 {
  io::print("Count is {count}")
  count =+ 1
}
```

## Match Expressions

Match expressions are similar to `switch` expressions in most languages.

### Integer Matching

When ranges overlap, the first match wins:

```ard
let grade = match score {
  0 => "How?",
  1..59 => "F",
  60..69 => "D",
  70..79 => "C",
  80..89 => "B",
  90..100 => "A",
  _ => "Invalid score"
}
```

### Boolean Matching

```ard
let response = match is_valid {
  true => "Proceed"
  false => "Error: invalid input"
}
```

### Enum Matching

```ard
enum Status { active, inactive, pending }

let message = match user_status {
  Status::active => "Welcome back!"
  Status::inactive => "Please reactivate account"
  Status::pending => "Account under review"
}
```

### Matching on Type Unions

Use match expressions to handle different types in a union:

```ard
type Content = Str | Int | Bool

fn describe(value: Content) Str {
  match value {
    Str => "Text: {it}"
    Int => "Number: {it.to_str()}"
    Bool => "Flag: {it.to_str()}"
  }
}

let items: [Content] = ["hello", 42, true]
for item in items {
  io::print(describe(item))
}
```

## Pattern Matching Order

Patterns are evaluated in the order they appear. More specific patterns should come before general ones:

## Loop Control

Ard supports the `break` keyword for early termination of loops.

```ard
for item in items {
  if should_skip(item) {
    break
  }
  process(item)
}
```
