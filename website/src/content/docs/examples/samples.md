---
title: Code Samples
description: Practical examples demonstrating current Ard language features.
---

These samples focus on core Ard syntax and the current Go backend. They use direct Go imports for output.

## Hello World

```ard
use go:fmt

fn main() {
  fmt::Println("Hello, World!")
}
```

## FizzBuzz

```ard
use go:fmt

fn label(num: Int) Str {
  match {
    num % 15 == 0 => "FizzBuzz"
    num % 3 == 0 => "Fizz"
    num % 5 == 0 => "Buzz"
    _ => num.to_str()
  }
}

fn main() {
  for num in 1..100 {
    fmt::Println(label(num))
  }
}
```

## Structs and Methods

```ard
use go:fmt

struct Todo {
  title: Str,
  completed: Bool,
}

impl Todo {
  fn text() Str {
    let box = match self.completed {
      true => "[x]"
      false => "[ ]"
    }
    "{box} {self.title}"
  }

  fn mut complete() {
    self.completed = true
  }
}

fn main() {
  mut todo = Todo{title: "Update docs", completed: false}
  todo.complete()
  fmt::Println(todo.text())
}
```

## Maybe Values

```ard
use go:fmt

fn find_name(id: Int) Str? {
  match id == 1 {
    true => Maybe::new("Ada")
    false => Maybe::new()
  }
}

fn main() {
  match find_name(1) {
    name => fmt::Println("Hello, {name}")
    _ => fmt::Println("Unknown user")
  }
}
```

## Result and `try`

```ard
use go:fmt

fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("cannot divide by zero")
    false => Result::ok(a / b)
  }
}

fn describe_division(a: Int, b: Int) Str!Str {
  let value = try divide(a, b)
  Result::ok("{a} / {b} = {value}")
}

fn main() {
  match describe_division(10, 2) {
    ok(text) => fmt::Println(text)
    err(message) => fmt::Println("Error: {message}")
  }
}
```

## Lists and Maps

```ard
use go:fmt

fn filter_evens(numbers: [Int]) [Int] {
  mut result: [Int] = []
  for num in numbers {
    if num % 2 == 0 {
      result.push(num)
    }
  }
  result
}

fn main() {
  let numbers = [1, 2, 3, 4, 5, 6]
  let evens = filter_evens(numbers)
  fmt::Println("Found {evens.size()} even numbers")

  mut counts: [Str:Int] = [:]
  counts.set("even", evens.size())
  fmt::Println("even: {counts.get("even").or(0)}")
}
```

## Fixed-Size Arrays

```ard
use go:crypto/sha256
use go:fmt

fn main() {
  mut bytes = "hello".bytes()
  let digest: [Byte; 32] = sha256::Sum256(bytes)
  let zero: Byte = 0
  fmt::Println("digest bytes: {digest.size()}")
  fmt::Println("first byte: {digest.at(0).or(zero)}")
}
```

## Direct Go Variadic Calls

```ard
use go:fmt

fn main() {
  fmt::Println()
  fmt::Println("count", 3, "ready", true)
}
```
