---
title: Code Samples
description: Practical examples demonstrating Ard language features and common programming patterns.
---

## Basic Examples

### Hello World

The simplest Ard program:

```ard
use ard/io

fn main() {
  io::print("Hello, World!")
}
```

### FizzBuzz

Classic FizzBuzz implementation showcasing control flow:

```ard
use ard/io

fn main() {
  for num in 1..100 {
    match num {
      n if n % 15 == 0 => io::print("FizzBuzz")
      n if n % 3 == 0 => io::print("Fizz")
      n if n % 5 == 0 => io::print("Buzz")
      _ => io::print(num.to_str())
    }
  }
}
```

### Fibonacci Sequence

Recursive function demonstrating pattern matching:

```ard
use ard/io

fn fibonacci(n: Int) Int {
  match n <= 1 {
    true => n
    false => fibonacci(n - 2) + fibonacci(n - 1)
  }
}

fn main() {
  for n in 1..20 {
    io::print("fib({n.to_str()}) = {fibonacci(n).to_str()}")
  }
}
```

## Struct Examples

### Todo List Application

Complete todo list with structs and methods:

```ard
use ard/io

struct Todo {
  title: Str
  completed: Bool
}

impl Todo {
  fn get_str() Str {
    let box = match @completed {
      true => "[x]"
      false => "[ ]"
    }
    "{box} {@title}"
  }
}

fn Todo::new(title: Str) Todo {
  Todo { title: title, completed: false }
}

fn render(list: [Todo]) {
  io::print("Todo List:")
  for todo in list {
    io::print(todo.get_str())
  }
}

fn main() {
  mut list: [Todo] = [
    Todo { title: "Buy milk", completed: true }
  ]

  mut running = true
  while running {
    render(list)
    io::print("What's your next todo?")
    let title = io::read_line().unwrap_or("")
    
    match title.is_empty() {
      true => running = false
      false => {
        list.push(Todo::new(title))
        io::print("------")
      }
    }
  }
}
```

## Type Union Examples

### Shape Calculator

Using type unions for polymorphic behavior:

```ard
use ard/io

struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn get_name(shape: Shape) Str {
  match shape {
    Square => "Square"
    Circle => "Circle"
  }
}

fn calculate_area(shape: Shape) Float {
  match shape {
    Square => (it.size * it.size).to_float()
    Circle => 3.14159 * (it.radius * it.radius).to_float()
  }
}

fn main() {
  let square = Square { size: 10 }
  let circle = Circle { radius: 5 }

  let shapes: [Shape] = [square, circle]
  for shape in shapes {
    let name = get_name(shape)
    let area = calculate_area(shape)
    io::print("{name} area: {area.to_str()}")
  }
}
```

## Trait Examples

### Custom Display Trait

Implementing traits for custom types:

```ard
use ard/io

struct Book {
  title: Str
  author: Str
}

impl String for Book {
  fn to_str() Str {
    "{@title} by {@author}"
  }
}

fn display(item: String) {
  io::print("Book: {item.to_str()}")
}

fn main() {
  let book = Book { 
    title: "The Hobbit", 
    author: "J.R.R. Tolkien" 
  }
  display(book)
}
```

## Error Handling Examples

### Safe Division

Demonstrating Result types and error handling:

```ard
use ard/io

fn safe_divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("Cannot divide by zero")
    false => Result::ok(a / b)
  }
}

fn calculate(operations: [(Int, Int)]) {
  for pair in operations {
    let a = pair.0
    let b = pair.1
    
    match safe_divide(a, b) {
      ok(result) => io::print("{a.to_str()} / {b.to_str()} = {result.to_str()}")
      err(error) => io::print("Error: {error}")
    }
  }
}

fn main() {
  let operations = [(10, 2), (15, 3), (8, 0), (20, 4)]
  calculate(operations)
}
```

### Chained Operations

Using the `try` keyword for error propagation:

```ard
use ard/io

fn parse_number(text: Str) Int!Str {
  text.to_int().or_err("Invalid number format")
}

fn calculate_percentage(text1: Str, text2: Str) Float!Str {
  let num1 = try parse_number(text1)
  let num2 = try parse_number(text2)
  
  match num2 == 0 {
    true => Result::err("Cannot divide by zero")
    false => Result::ok((num1.to_float() / num2.to_float()) * 100.0)
  }
}

fn main() {
  let inputs = [("75", "100"), ("50", "200"), ("abc", "100"), ("75", "0")]
  
  for pair in inputs {
    match calculate_percentage(pair.0, pair.1) {
      ok(percentage) => io::print("{pair.0}/{pair.1} = {percentage.to_str()}%")
      err(error) => io::print("Error with {pair.0}/{pair.1}: {error}")
    }
  }
}
```

## Collection Examples

### List Processing

Working with lists and functional patterns:

```ard
use ard/io

fn filter_evens(numbers: [Int]) [Int] {
  let mut result: [Int] = []
  for num in numbers {
    if num % 2 == 0 {
      result.push(num)
    }
  }
  result
}

fn sum_list(numbers: [Int]) Int {
  mut total = 0
  for num in numbers {
    total =+ num
  }
  total
}

fn main() {
  let numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]
  
  io::print("Original: {numbers.to_str()}")
  
  let evens = filter_evens(numbers)
  io::print("Even numbers: {evens.to_str()}")
  
  let sum = sum_list(evens)
  io::print("Sum of evens: {sum.to_str()}")
}
```

### Map Operations

Using maps for data storage and lookup:

```ard
use ard/io

fn word_count(text: Str) [Str:Int] {
  let words = text.split(" ")
  let mut counts: [Str:Int] = [:]
  
  for word in words {
    let current = counts.get(word).or(0)
    counts.set(word, current + 1)
  }
  
  counts
}

fn main() {
  let text = "the quick brown fox jumps over the lazy dog the fox"
  let counts = word_count(text)
  
  io::print("Word frequencies:")
  for word, count in counts {
    io::print("{word}: {count.to_str()}")
  }
}
```

## Server Example

### Simple HTTP Server

Basic HTTP server using the `ard/http` module:

```ard
use ard/http
use ard/io

fn handle_home(req: http::Request) http::Response {
  http::Response::new(200, "Welcome to Ard server!")
}

fn handle_about(req: http::Request) http::Response {
  let content = "This is an Ard-powered web server running on {req.host()}"
  http::Response::new(200, content)
}

fn handle_echo(req: http::Request) http::Response {
  let path = req.path().unwrap_or("/")
  http::Response::new(200, "You requested: {path}")
}

fn main() {
  io::print("Starting server on port 3000...")
  
  http::serve(3000, [
    "/": handle_home,
    "/about": handle_about,
    "/echo": handle_echo
  ])
}
```

## Pattern Matching Examples

### State Machine

Using enums and pattern matching for state management:

```ard
use ard/io

enum TrafficLight {
  red,
  yellow,
  green
}

fn next_light(current: TrafficLight) TrafficLight {
  match current {
    TrafficLight::red => TrafficLight::green
    TrafficLight::yellow => TrafficLight::red
    TrafficLight::green => TrafficLight::yellow
  }
}

fn light_duration(light: TrafficLight) Int {
  match light {
    TrafficLight::red => 30
    TrafficLight::yellow => 5
    TrafficLight::green => 25
  }
}

fn main() {
  mut current = TrafficLight::red
  
  for cycle in 1..10 {
    let duration = light_duration(current)
    let name = match current {
      TrafficLight::red => "RED"
      TrafficLight::yellow => "YELLOW"
      TrafficLight::green => "GREEN"
    }
    
    io::print("Cycle {cycle.to_str()}: {name} light for {duration.to_str()} seconds")
    current = next_light(current)
  }
}
```

These examples demonstrate the core features of Ard and common programming patterns. Each example focuses on specific language features while solving practical problems.