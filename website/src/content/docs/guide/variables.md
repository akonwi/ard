---
title: Variables
description: Learn about variable declaration, mutability, and type inference in Ard.
---

## Declaration Keywords

Ard uses two keywords for variable declaration:

- `let` for immutable bindings
- `mut` for mutable bindings

## Type Inference

Variable types can be inferred from their initial values:

```ard
let count = 42
let pi = 3.14
let greeting = "Hello"
let active = true
```

## Explicit Type Annotations

Types can be optionally be declared:

```ard
let name: Str = "Bob"
let temperature: Float = 98.6
let items: [Int] = [1, 2, 3]
let map: [Str:Int] = ["a": 1, "b": 2]
```

## Immutability with `let`

An immutable binding is read-only meaning it can neither be reassigned or mutated:

```ard
let x = 10
x = 20        // Error: cannot reassign immutable variable

let numbers = [1, 2, 3]
numbers.push(4)  // Error: cannot mutate immutable variable
```

## Mutability with `mut`

Variables declared with `mut` can be modified:

```ard
mut counter = 0
counter = 5           // OK
counter =+ 1          // OK, increment by 1
```

## Increment and Decrement

Ard uses a unique syntax for compound assignment operators, placing the `=` first for left-to-right readability:

```ard
mut value = 10

value =+ 5    // Equivalent to value = value + 5
value =- 2    // Equivalent to value = value - 2
```

There are no `++` or `--` operators in Ard and only increment (=+) and decrement (=-) are supported.

## Scoping Rules

Variables follow standard block scoping rules:

```ard
let outer = "global"

if some_condition {
  let inner = "local"
  // Both 'outer' and 'inner' are accessible here
}

// Only 'outer' is accessible here
// 'inner' is out of scope
```

## Shadowing

Redeclaring a variable with the same name in the same scope is allowed.
This acts as a wipe of the binding and is only valid if their usages are consisten.

```ard
let x = 5
let x = x + 1    // Creates new variable, x is now 6
let x: Str = "hello"  // Creates new variable with different type
x.size() // x is now a string and can only be used as a string
```
