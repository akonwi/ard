# Ard Lang

## What's Ard?
 - __A__ __R__-eally __D__-ope Language
 - [ard is slang for “alright.”](https://www.dictionary.com/e/slang/ard/)
 - [Irish and Scottish Gaelic word meaning 'high, lofty', 'above the ground, elevated](https://www.oxfordreference.com/display/10.1093/oi/authority.20110803095422661)
 - Ardbeg is my favorite scotch and I was drinking it when I came up with this name

## Language Description

Ard is a modern, statically-typed programming language designed for clarity, safety, and ease.
It combines features from JavaScript, Swift, and Go while introducing its own unique characteristics.

## Goals

- **Readability**: Ard code should be easy to read and understand.
- **Simple**: There should be one obvious way to do things.
- **Safety**: The compiler should catch errors at compile time.
- **Reliable**: Runtime is in Go, so it's fast and efficient.
  - [Future] Compiles to Go for portability.

## Basic Syntax

Ard uses a clean, expressive syntax designed for readability and ease of use.
Note: trying to follow Go's philosophy for readablity left to right, rather than usual Spiraling in C based syntax.

### Variables and Constants

- Use `let` for constants and `mut` for variables
- `let` variables cannot be reassigned or mutated
- Variable types can be inferred or explicitly declared

```ard
let name: Str = "Alice"
mut age = 30
```

#### Increment/Decrement short hand

The syntax for this is slightly different from other languages.
Rather than '+=' or '-=', in Ard, the `=` comes first (left to right readability)

```ard
age =+ 1
age =- 2
```

There is no `++` or `--`.

### Functions

- Use `fn` keyword to define functions
- Return type is specified after the parameter list
  - Return types are required. Without a return type, the function return is `Void` and ignored
- There is no return keyword. The last expression is the returned value

```ard
fn greet(name: Str) Str {
  "Hello, {{name}}!"
}
```

### Control Flow

Ard supports common control flow structures:

```ard
if some_condition {
    // ...
} else if another_condition {
    // ...
} else {
    // ...
}

for item in array {
  // ...
}

// check condition, then do block
while condition {
  // ...
}

// match expressions, similar to a switch statements
let string = match some_bool {
  true => "It's true"
  false => "It's false"
}
```

### Iteration

C style for loop:

```ard
for mut i = 0; i <= 5; i =+1 {
  io.print(i.to_str())
}
```

Numeric inclusive range:
```ard
for i in 1..10 {
	io.print(i) // 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
}
```

Iterating over a list:
```ard
let fruits = ["apple", "banana", "cherry"]
for fruit in fruits {
  io.print(fruit)
}
```
## Types

### Built-in types
- Str
- Int
- Float
- Bool
- [Int] - List
- [Str:Int] - Map

### Structs

Structs can be used to define a collection of arbitrary data types, i.e. objects:

```ard
struct Person {
  name: Str
  age: Int
}

let person = Person {
  name: "Alton",
  age: 30
}
person.name // "Alton"
```

Structs can have methods. Use a `impl` block to define methods on a struct:

```ard
impl (p: Person) {
  fn get_intro() Str {
    "My name is {{p.name}}"
  }

  fn greet(other: Person) Str {
    "Hello, {{other.name}}"
  }
}

person.get_intro() // "My name is Alton"
```

### Enums

Enums are used to enumerate a specific set of values.
They are simply labeled numbers.
They cannot have associated values.

```ard
enum Status {
  active,
  inactive,
  pending
}
```

The static operator (`::`) is used to cccess variants.
The static operator avoids naming conflicts between the variants and instance properties on the enum.

Enum values can be used in match expressions to handle different cases:
```ard
let status = Status::inactive
match status {
  Status::active => "Active",
  Status::inactive => "Inactive",
  Status::pending => "Pending"
}
```

### Optional Types

Optional type declarations end with `?` and can be created using the `ard/option` package from the standard library.
An optional type can either have a value (`some`) or be empty (`none`).

```ard
use ard/option

mut maybe_name: Str? = option.none()
maybe_name = option.some("Joe")

match maybe_name {
  n => "Hello, {{n}}",
  _ => "Hello, stranger"
}
```

### Type Unions

Type unions are used to define a type that can be one of several types.

```ard
type Printable = Str | Int
let value: Printable = "Hello"
let stuff: [Printable] = ["Hello", 42]
```

To do conditional logic on a value of a type union, use a match expression and within each case, the value is bound to a variable called `it`:

```ard
for item in stuff {
  match item {
    Str => io.print("String: {{it}}"),
    Int => io.print("Number: {{it.to_str()}}")
  }
}
```

#### 👇🏿 everything below this line is a work in progress 👇🏿

### TODO: Callbacks
- could be a way to handle async return values as an attached statement
  - side-effecty, no control over when it's executed

```ard
greet("John") => (msg) {
  print "Received $msg"
}
```

## 3. Error Handling

Ard uses a unique error handling mechanism centered around the `else` keyword.

### Throwing Functions

Functions that can throw errors must be marked with `throws` in their signature:

```ard
func riskyOperation() throws -> String {
    if someCondition {
        throw Error("Operation failed")
    }
    return "Success"
}
```

### Handling Errors

The `else` keyword is used after potentially throwing operations to handle errors. It can be used in two forms:

1. Early Return or Throw Block:

```ard
func processData() -> String {
    let data = riskyOperation() else {
        return "Failed to process data"
    }
    return "Processed: " + data
}
```

2. Fallback Value:

```ard
let username: String = getUserName() else "Anonymous"
```

### Using `try` and `try?`

The `try` keyword is used to propagate errors up the call stack:

```ard
func performOperation() throws {
    let result = try riskyOperation()
    console.log(result)
}
```

The `try?` operator can be used to convert a throwing expression to an optional:

```ard
let result: String? = try? riskyOperation()
```

## 4. Asynchronous Programming

Ard uses the `async` and `await` keywords for handling asynchronous operations.

### Async Functions

Async functions are declared using the `async` keyword:

```ard
async func fetchData() -> String {
    // implementation
}
```

### Await and Error Handling

The `await` keyword signifies a JavaScript Promise. All `await` expressions require an `else` block for error handling, unless used with `try await` or `try? await`.

```ard
async func getData() -> String {
    let data = await fetchData() else {
        return "Failed to fetch data"
    }
    return data
}

async func riskyGet() throws -> String {
    let data = try await fetchData()
    return data
}
```

## 5. Module System

### Module Definition and Exports

In Ard, every file is implicitly a module. By default, all declarations in a file are exported and available to other modules.

Use the `internal` keyword before any declaration to make it private to the module:

```ard
internal let secretKey = "1234567890"
internal func helperFunction() {
    // This function is only available within this module
}
```

### Import Syntax and Mechanisms

Ard uses the `import` keyword to bring declarations from other files into the current scope.

Basic import syntax:

```ard
import { functionName, TypeName } from "filename"
```

The compiler will look for a file named `filename.ard` in the same directory as the current file.

Renaming imports:

```ard
import { originalName: newName } from "filename"
```

This specification provides an overview of the Ard language. It covers the basic syntax, type system, error handling, asynchronous programming, pattern matching, and module system. As the language evolves, this specification will be updated to reflect new features and changes.
