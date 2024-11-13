# Kon Language Specification

Kon is a modern, statically-typed programming language designed for clarity, safety, and expressiveness.
It combines features from JavaScript, Swift, and Go while introducing its own unique characteristics.

## 1. Basic Syntax

Kon uses a clean, expressive syntax designed for readability and ease of use.
Note: trying to follow Go's philosophy for readablity left to right, rather than usual Spiraling in C based syntax.

### Built-in types
- Str
- Num
- Bool
- [Num] - List
- [Str:Num] - Map
- Void - non-existence
  - can only be used to indicate a function does not return a value

### Variables and Constants

- Use `let` for constants and `mut` for variables
- `let` variables cannot be reassigned or mutated
- Variable types can be inferred or explicitly declared

```kon
let name: Str = "Alice"
mut age = 30
```

### Functions

- Use `fn` keyword to define functions
- Return type is specified after the parameter list

```kon
fn greet(name: Str) Str {
  return "Hello, ${name}!"
}
```

### Control Flow

Kon supports common control flow structures:

```kon
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

// do block, then check condition
do while condition {
  // ...
}
```

### Iteration

Numeric inclusive range:
```kon
for i in 1...10 {
	print(i)
}

// idea for changing step size
for i in 1...10 by 2 {
	print(i)
}
```
### TODO: Callbacks
- could be a way to handle async return values as an attached statement
  - side-effecty, no control over when it's executed

```kon
greet("John") => (msg) {
  print "Received $msg"
}
```
## 2. Types

### Structs

Structs can be used to define objects with properties:

```kon
struct Person {
  name: Str
  age: Num
}
```

### Enums

Enums are used to define a type that can only have a specific set of values:

```kon
enum Status {
  active,
  inactive,
  pending
}
```

Enums can also have associated values:

```kon
enum Result {
  success(Num),
  failure(Str)
}
```

### Optional Types

Optional types are represented using the built-in `Option` enum:

```kon
enum Option<T> {
  some(T),
  none
}

mut maybe_name: Option<String> = Option::some("Alice")
maybe_name = Option::none()
```

#### 👇🏿 everything below this line is a work in progress 👇🏿

## 3. Error Handling

Kon uses a unique error handling mechanism centered around the `else` keyword.

### Throwing Functions

Functions that can throw errors must be marked with `throws` in their signature:

```kon
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

```kon
func processData() -> String {
    let data = riskyOperation() else {
        return "Failed to process data"
    }
    return "Processed: " + data
}
```

2. Fallback Value:

```kon
let username: String = getUserName() else "Anonymous"
```

### Using `try` and `try?`

The `try` keyword is used to propagate errors up the call stack:

```kon
func performOperation() throws {
    let result = try riskyOperation()
    console.log(result)
}
```

The `try?` operator can be used to convert a throwing expression to an optional:

```kon
let result: String? = try? riskyOperation()
```

## 4. Asynchronous Programming

Kon uses the `async` and `await` keywords for handling asynchronous operations.

### Async Functions

Async functions are declared using the `async` keyword:

```kon
async func fetchData() -> String {
    // implementation
}
```

### Await and Error Handling

The `await` keyword signifies a JavaScript Promise. All `await` expressions require an `else` block for error handling, unless used with `try await` or `try? await`.

```kon
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

Kon supports pattern matching using the `match` expression:

```kon
match value {
    case pattern1 -> expression1
    case pattern2 -> expression2
    _ -> defaultExpression
}
```

Example:

```kon
match point {
    case (0, 0) -> "Origin"
    case (x, 0) -> "On x-axis at " + x
    case (0, y) -> "On y-axis at " + y
    case (x, y) -> "At (" + x + ", " + y + ")"
}
```

## 6. Module System

### Module Definition and Exports

In Kon, every file is implicitly a module. By default, all declarations in a file are exported and available to other modules.

Use the `internal` keyword before any declaration to make it private to the module:

```kon
internal let secretKey = "1234567890"
internal func helperFunction() {
    // This function is only available within this module
}
```

### Import Syntax and Mechanisms

Kon uses the `import` keyword to bring declarations from other files into the current scope.

Basic import syntax:

```kon
import { functionName, TypeName } from "filename"
```

The compiler will look for a file named `filename.kon` in the same directory as the current file.

Renaming imports:

```kon
import { originalName: newName } from "filename"
```

This specification provides an overview of the Kon language. It covers the basic syntax, type system, error handling, asynchronous programming, pattern matching, and module system. As the language evolves, this specification will be updated to reflect new features and changes.
