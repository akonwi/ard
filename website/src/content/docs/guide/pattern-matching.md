---
title: Pattern Matching
description: Learn about Ard's powerful pattern matching with match expressions.
---

## Match Expressions

Match expressions provide powerful pattern matching capabilities in Ard. They are similar to switch statements but are more expressive:

```ard
let is_ready = false
let label = match is_ready {
  true => "ready",
  false => "waiting",
}
```

## Exhaustiveness

Match expressions must be exhaustive and handle all possibilities.

The `_` wildcard matches serves as a catch-all after specific cases.

## Integer Patterns

### Specific Values

```ard
let status_code = 200

let message = match status_code {
  200 => "OK",
  404 => "Not Found",
  500 => "Server Error",
  _ => "Unknown Status",
}
```

### Range Patterns

Integer ranges are inclusive and use the `..` syntax:

```ard
let score = 85

let grade = match score {
  0..59 => "F",
  60..69 => "D",
  70..79 => "C",
  80..89 => "B",
  90..100 => "A",
  _ => "Invalid score",
}
```

### Mixed Patterns

Combine specific values and ranges:

```ard
let age = 30

let category = match age {
  0 => "newborn",
  1..12 => "child",
  13..19 => "teenager",
  21 => "legal drinking age",
  22..64 => "adult",
  65..120 => "senior",
  _ => "invalid age",
}
```

**Important**: When patterns overlap, the first match wins. Order patterns from most specific to least specific:

```ard
let number = 42

// Good: specific cases first
let good = match number {
  42 => "the answer",    // Specific case
  40..50 => "forties",   // Range that includes 42
  _ => "other",
}

// Problematic: general case first
let bad = match number {
  40..50 => "forties",   // This catches 42
  42 => "the answer",    // This will never match
  _ => "other",
}
```

## String Patterns

Match `Str` values with string literal cases. Because strings are open-ended, string matches require a `_` catch-all case.

```ard
let extension = ".md"

let kind = match extension {
  ".md" => "markdown",
  ".html" => "html",
  _ => "unknown",
}
```

String patterns use exact equality. Duplicate string cases are rejected.

## Boolean Patterns

Boolean matches are like Ard's version of ternary expressions in other languages.

```ard
let is_admin = true
let logged_in = true
let verified = false

let access_level = match is_admin {
  true => "full access",
  false => "limited access",
}

// More complex boolean logic
let message = match logged_in and verified {
  true => "Welcome to your account",
  false => "Please log in and verify your account",
}
```

## Matching Enums

```ard
enum Priority {
  low,
  medium,
  high,
  critical,
}

let task_priority = Priority::high

let urgency = match task_priority {
  Priority::low => "Can wait",
  Priority::medium => "Normal priority",
  Priority::high => "Important",
  Priority::critical => "Urgent!",
}
```

## Matching on Type Unions

```ard
use go:fmt

type Content = Str | Int | Bool

fn describe(value: Content) Str {
  match value {
    Str(string) => "Text: '{string}'",
    Int(i) => "Number: {i}",
    Bool(val) => "Boolean: {val}",
  }
}

// Usage
let items: [Content] = ["hello", 42, true]
for item in items {
  fmt::Println(describe(item))
}
// Output:
// Text: 'hello'
// Number: 42
// Boolean: true
```

## Matching on Maybes

```ard

let maybe_name: Str? = Maybe::new("Alice")

let greeting = match maybe_name {
  name => "Hello, {name}!",     // Binds the value if present
  _ => "Hello, stranger!",      // Matches none case
}
```

## Matching on Results

```ard
fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("Division by zero"),
    false => Result::ok(a / b),
  }
}

let result = divide(10, 2)
let message = match result {
  ok(value) => "Result: {value}",
  err(error) => "Error: {error}",
}
```

## Complex Matching Examples

### State Machine

```ard
enum State {
  idle,
  running,
  paused,
  error,
}

enum Event {
  start,
  pause,
  resume,
  stop,
  fail,
}

fn handle_event(current_state: State, event: Event) State {
  match current_state {
    State::idle => match event {
      Event::start => State::running,
      _ => State::idle,
    },
    State::running => match event {
      Event::pause => State::paused,
      Event::stop => State::idle,
      Event::fail => State::error,
      _ => State::running,
    },
    State::paused => match event {
      Event::resume => State::running,
      Event::stop => State::idle,
      _ => State::paused,
    },
    State::error => match event {
      Event::start => State::running,
      _ => State::error,
    },
  }
}
```
