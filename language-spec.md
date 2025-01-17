# Ard Language Specification

Ard is a modern, statically-typed programming language designed for clarity, safety, and ease.
It combines features from JavaScript, Swift, and Go while introducing its own unique characteristics.

## Goals

- **Readability**: Ard code should be easy to read and understand.
- **Simple**: There should be one obvious way to do things.
- **Safety**: The compiler should catch errors at compile time.
- **Reliable**: Runtime is in Go, so it's fast and efficient.
  - [Future] Compiles to Go for portability.

## 1. Basic Syntax

Ard uses a clean, expressive syntax designed for readability and ease of use.
Note: trying to follow Go's philosophy for readablity left to right, rather than usual Spiraling in C based syntax.

### Built-in types
- Str
- Num
- Bool
- [Num] - List
- [Str:Num] - Map
- Void - non-existence
  - only used to discard a function's return value

### Variables and Constants

- Use `let` for constants and `mut` for variables
- `let` variables cannot be reassigned or mutated
- Variable types can be inferred or explicitly declared

```ard
let name: Str = "Alice"
mut age = 30
```

### Functions

- Use `fn` keyword to define functions
- Return type is specified after the parameter list
  - Return types are required. Without a return type, the function returns `Void`

```ard
fn greet(name: Str) Str {
  return "Hello, {{name}}!"
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
```

### Iteration

Numeric inclusive range:
```ard
for i in 1..10 {
	io.print(i)
}

// todo: more traditional for loop
for mut i = 1; i < 10; i =+2; {
	io.print(i)
}
```
### TODO: Callbacks
- could be a way to handle async return values as an attached statement
  - side-effecty, no control over when it's executed

```ard
greet("John") => (msg) {
  print "Received $msg"
}
```
## 2. Types

### Structs

Structs can be used to define objects with properties:

```ard
struct Person {
  name: Str
  age: Num
}
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

The static operator (`::`) is used to cccess variants. For example:
The static operator avoids naming conflicts between the variants and instance properties on the enum.

```ard
Status::inactive
```

### Optional Types

Optional types are represented using the built-in `Option`:

```ard
Option {
  some(T?),
  none
}

mut maybe_name: Option<Str> = Option::some("Alice")
maybe_name = Option::none()
```

#### 👇🏿 everything below this line is a work in progress 👇🏿

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

## 5. Pattern Matching

Ard supports pattern matching using the `match` expression:

```ard
match value {
    case pattern1 -> expression1
    case pattern2 -> expression2
    _ -> defaultExpression
}
```

Example:

```ard
match point {
    case (0, 0) -> "Origin"
    case (x, 0) -> "On x-axis at " + x
    case (0, y) -> "On y-axis at " + y
    case (x, y) -> "At (" + x + ", " + y + ")"
}
```

## 6. Module System

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

### 7. Generics
Generic types end with a `?`.

```ard
fn map(item: A?) Str {
  "foo"
}
```
