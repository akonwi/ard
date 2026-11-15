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

## How Arms Are Chosen

Arms are tried in the order they are written, and the first arm whose pattern matches runs. The subject is evaluated once, before any arm is tried.

Two patterns match any value: `_`, which ignores it, and a name, which binds it for the arm's body.

```ard
let code = 503

let message = match code {
  200 => "OK",
  404 => "Not Found",
  other => "Unexpected status {other}",
}
```

## Exhaustiveness

Match expressions must be exhaustive: every possible value has to be matched by some arm. When one is missing, the compiler lists the cases that no arm covers:

```text
error: Non-exhaustive match
 --> main.ard:2:3
  |
2 |   match (left, right) {
  |   ^^^^^^^^^^^^^^^^^^^^^ add a case for `(ok(_), err(_))`
```

`Bool`, enums, `Maybe`, `Result`, and type unions have a fixed set of cases, so listing them all is enough. `Int`, `Rune`, and `Str` values are open-ended, so a match on them needs a `_` or a name to catch the rest.

An arm that can never run, because earlier arms already match every value it matches, is reported as an unreachable arm. This includes repeated cases and anything after a catch-all.

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
  42 => "the answer",    // Unreachable: reported as a warning
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

String patterns use exact equality. A repeated string case is unreachable and reported as a warning.

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

A `_` arm can stand in for the remaining case.

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

Union members imported through a module or Go package use qualified patterns. The binding receives the exact member type, including pointer types:

```ard
use go:database/sql

type QueryTarget = &mut sql::DB | &mut sql::Tx

fn close(target: QueryTarget) Void!Error {
  match target {
    sql::DB(db) => db.Close(),
    sql::Tx(tx) => tx.Rollback(),
  }
}
```

Omit the binding to use the implicit `it` variable, such as `sql::DB => it.Close()`. A union containing both `T` and `&mut T` requires a different representation because the qualified pattern name would be ambiguous.

## Matching on Maybes

`some(pattern)` matches a present value and `none` matches an absent one:

```ard
let maybe_name: Str? = Maybe::new("Alice")

let greeting = match maybe_name {
  some(name) => "Hello, {name}!",
  none => "Hello, stranger!",
}
```

A match on a whole `Maybe` can also use the shorter form, where a name binds the present value and `_` matches the absent one:

```ard
let maybe_name: Str? = Maybe::new("Alice")

let greeting = match maybe_name {
  name => "Hello, {name}!",     // Binds the value if present
  _ => "Hello, stranger!",      // Matches none case
}
```

The shorter form only applies to the whole subject. Inside a [tuple pattern](#matching-on-tuples), a name binds the whole element and `_` matches anything, so use `some(...)` and `none` there.

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

The qualified spellings `Result::ok(value)` and `Result::err(error)` work too. A bare `ok` or `err` binds the value under that name, as in `ok => "Result: {ok}"`.

## Nested Patterns

The pattern inside `ok(...)`, `err(...)`, `some(...)`, or a union member such as `Int(...)` can itself be any pattern for the value it holds, not only a name:

```ard
fn describe(response: Int!Str) Str {
  match response {
    ok(200) => "OK",
    ok(code) => "Unexpected status {code}",
    err("timeout") => "Try again",
    err(message) => "Failed: {message}",
  }
}
```

## Matching on Tuples

A tuple can be matched with tuple patterns: a parenthesized, comma-separated list with one pattern per element. Each element pattern is any pattern for that element's type, `_`, or a name that binds the element. This makes decisions over several values at once:

```ard
fn add(left: Int!Str, right: Int!Str) Int!Str {
  match (left, right) {
    (ok(a), ok(b)) => Result::ok(a + b),
    (err(e), _) => Result::err(e),
    (_, err(e)) => Result::err(e),
  }
}
```

The subject can be any tuple, including the results of a call:

```ard
fn fetch() (Int, Str) {
  (200, "hello")
}

let text = match fetch() {
  (200, body) => body,
  (404, _) => "not found",
  (code, _) => "failed with {code}",
}
```

```ard
fn same_index(left: Int?, right: Int?) Bool {
  match (left, right) {
    (some(a), some(b)) => a == b,
    (none, none) => true,
    _ => false,
  }
}
```

A tuple pattern must have exactly as many elements as the tuple, and tuple patterns nest for tuples inside tuples. When the subject is written as a tuple, like `(left, right)`, its elements are evaluated once each, left to right.

Exhaustiveness covers combinations: `(true, _)` and `(false, _)` together cover a `(Bool, Str)`, and a missing combination is reported as a pattern such as `(false, _)`.

Inside a tuple pattern, constructor patterns spell out their value: write `ok(_)` rather than `ok`, and `Str(_)` or `Str(text)` rather than a bare union member name.

