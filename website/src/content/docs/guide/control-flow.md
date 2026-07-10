---
title: Control Flow
description: Learn about conditional statements, loops, and pattern matching in Ard.
---

## Conditional Statements

### If-Else

```ard
use go:fmt

let temperature = 22

if temperature > 30 {
  fmt::Println("It's hot!")
} else if temperature < 10 {
  fmt::Println("It's cold!")
} else {
  fmt::Println("Nice weather!")
}
```

Conditions must be boolean expressions. There are no implicit truthy/falsy coercions. Comparison operators include `==`, `!=`, `<`, `<=`, `>`, and `>=`; combine boolean expressions with `and`, `or`, and `not`.

## Loops

### For Loops

#### Iterating Over Collections

```ard
use go:fmt

let fruits = ["apple", "banana", "cherry"]
for fruit, index in fruits {
  fmt::Println("{index}: {fruit}")
}
```

The index cursor can be omitted in list loops

```ard
use go:fmt

let fruits = ["apple", "banana", "cherry"]
for fruit in fruits {
  fmt::Println(fruit)
}
```

#### Iterating Over Maps

```ard
use go:fmt

let scores: [Str:Int] = ["Alice": 95, "Bob": 87, "Carol": 92]
for name, score in scores {
  fmt::Println("{name} scored {score.to_str()}")
}
```

#### Numeric Ranges

```ard
use go:fmt

// Inclusive range
for i in 1..10 {
  fmt::Println(i)
}
```

To iterate with a step other than 1, use a C-style loop:

```ard
use go:fmt

for mut i = 0; i <= 100; i =+ 10 {
  fmt::Println(i)  // Prints 0, 10, 20, ..., 100
}
```

#### C-Style For Loop

```ard
use go:fmt

for mut i = 0; i <= 5; i =+ 1 {
  fmt::Println("Count: {i}")
}
```

### While Loops

```ard
use go:fmt

mut count = 0
while count < 10 {
  fmt::Println("Count is {count}")
  count =+ 1
}
```

## Match Expressions

Match expressions are similar to `switch` expressions in most languages. They come in two forms: **value matching** (with a subject) and **conditional matching** (without a subject).

### Conditional Matching

Match expressions can be used without a subject to create clean conditional logic as an alternative to if-else chains:

```ard
let score = 85

let grade = match {
  score >= 90 => "A",
  score >= 80 => "B",
  score >= 70 => "C",
  score >= 60 => "D",
  _ => "F",
}
```

This is equivalent to, but more concise than, the if-else chain:

```ard
fn grade(score: Int) Str {
  if score >= 90 {
    "A"
  } else if score >= 80 {
    "B"
  } else if score >= 70 {
    "C"
  } else if score >= 60 {
    "D"
  } else {
    "F"
  }
}
```

Note that `if` blocks produce a value only as the final expression of a function body; they cannot be assigned directly with `let x = if ...`. Conditional `match` is the expression form.

Conditional match expressions evaluate conditions in order and execute the first matching case. **A catch-all case (`_`) is required** to ensure the expression always returns a value.

#### Complex Conditions

You can use any boolean expressions as conditions:

```ard
let age = 30
let has_license = true
let has_insurance = true

let status = match {
  age < 16 => "Too young to drive",
  not has_license => "Need to get a license",
  not has_insurance => "Need insurance",
  age >= 65 => "Senior driver",
  _ => "Ready to drive",
}

let temperature = 75
let sunny = true
let weekend = false

let activity = match {
  temperature > 80 and sunny => "Go to the beach",
  temperature > 70 and weekend => "Have a picnic",
  temperature < 50 => "Stay inside and read",
  _ => "Go for a walk",
}
```

### Integer Matching

When ranges overlap, the first match wins:

```ard
let score = 85

let grade = match score {
  0 => "How?",
  1..59 => "F",
  60..69 => "D",
  70..79 => "C",
  80..89 => "B",
  90..100 => "A",
  _ => "Invalid score",
}
```

### Boolean Matching

```ard
let is_valid = true

let response = match is_valid {
  true => "Proceed",
  false => "Error: invalid input",
}
```

### Enum Matching

```ard
enum Status {
  active,
  inactive,
  pending,
}

let user_status = Status::active

let message = match user_status {
  Status::active => "Welcome back!",
  Status::inactive => "Please reactivate account",
  Status::pending => "Account under review",
}
```

### Matching on Type Unions

Use match expressions to handle different types in a union:

```ard
use go:fmt

type Content = Str | Int | Bool

fn describe(value: Content) Str {
  match value {
    Str => "Text: {it}",
    Int => "Number: {it.to_str()}",
    Bool => "Flag: {it.to_str()}",
  }
}

let items: [Content] = ["hello", 42, true]
for item in items {
  fmt::Println(describe(item))
}
```

## Pattern Matching Order

Patterns are evaluated in the order they appear. More specific patterns should come before general ones.

## When to Use Match vs If-Else

**Use conditional match expressions when:**
- You need to return a value based on conditions
- You want cleaner, more functional code
- The logic is pure (no side effects)

**Use if statements when:**
- You need to perform side effects (like `break`, `panic`, or mutations)
- You're doing control flow within loops
- You're executing statements rather than returning values

```ard
struct User {}

impl User {
  fn is_admin() Bool { true }
  fn is_member() Bool { true }
}

fn should_skip(item: Int) Bool { false }
fn process(item: Int) {}

let user = User{}
let items = [1, 2, 3]

// Good: conditional match for values
let message = match {
  user.is_admin() => "Admin access granted",
  user.is_member() => "Member access granted",
  _ => "Access denied",
}

// Good: if statement for control flow
for item in items {
  // break is a side effect - can't use match here
  if should_skip(item) {
    break
  }
  process(item)
}
```

## Loop Control

Ard supports the `break` keyword for early termination of loops.

```ard
fn should_skip(item: Int) Bool { false }
fn process(item: Int) {}

let items = [1, 2, 3]

for item in items {
  if should_skip(item) {
    break
  }
  process(item)
}
```

## Deferred Cleanup

Use `defer` to schedule cleanup work for the end of the current function, method, closure, or script. Deferred work runs in last-in-first-out order and still runs when `try` returns early.

```ard
fn read_file(path: Str) Str!Str {
  let file = try open_file(path)
  defer file.close()

  let text = try file.read_all()
  Result::ok(text)
}
```

Both call and block forms are supported:

```ard
defer resource.close()

defer {
  match resource.close() {
    Result::err(e) => log("close failed: {e}"),
    Result::ok(_) => (),
  }
}
```

Unlike Go, Ard evaluates the deferred call later by lowering it as a zero-argument closure. If a deferred call captures a `mut` binding that is reassigned before the function exits, it sees the later value. Bind an explicit snapshot when needed:

```ard
let current = resource
defer current.close()
resource = next_resource
```

`try` is not allowed inside deferred work. Handle cleanup results explicitly with `match` if they matter.
