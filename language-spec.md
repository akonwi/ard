# Stone Language Specification

Stone is a modern, statically-typed programming language designed for clarity, safety, and expressiveness. It combines features from languages like Swift and JavaScript, while introducing its own unique characteristics.

## 1. Basic Syntax

Stone uses a clean, expressive syntax designed for readability and ease of use.

### Variables and Constants

- Use `let` for constants and `var` for variables
- Type inference is supported, but types can be explicitly declared

```stone
let name: String = "Alice"
var age = 30  // Type inferred as Int
```

### Functions

- Use `func` keyword to define functions
- Return type is specified after `->`

```stone
func greet(person: String) -> String {
    return "Hello, " + person + "!"
}
```

### Control Flow

Stone supports common control flow structures:

```stone
if condition {
    // code
} else if anotherCondition {
    // code
} else {
    // code
}

for item in array {
    // code
}

while condition {
    // code
}
```

## 2. Types

### Basic Types

- `Int`: Represents integer numbers
- `Double`: Represents floating-point numbers
- `String`: Represents textual data
- `Boolean`: Represents true or false values
- `Array<T>`: Represents an array of type T
- `Any`: Represents any type, including functions and objects
- `Void`: Represents the absence of a return value

### Custom Types

Use `struct` keyword to define custom types:

```stone
struct Person {
    let name: String
    var age: Int
}
```

### Enums

Enums are used to define a type that can only have a specific set of values:

```stone
enum Status {
    case active
    case inactive
    case pending
}
```

Enums can also have associated values:

```stone
enum Result<T, E> {
    case success(T)
    case failure(E)
}
```

### Optional Types

Optional types are represented using the `Option<T>` enum:

```stone
enum Option<T> {
    case some(T)
    case none
}

var optionalName: Option<String> = .some("Alice")
optionalName = .none  // Valid
```

## 3. Error Handling

Stone uses a unique error handling mechanism centered around the `else` keyword.

### Throwing Functions

Functions that can throw errors must be marked with `throws` in their signature:

```stone
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

```stone
func processData() -> String {
    let data = riskyOperation() else {
        return "Failed to process data"
    }
    return "Processed: " + data
}
```

2. Fallback Value:

```stone
let username: String = getUserName() else "Anonymous"
```

### Using `try` and `try?`

The `try` keyword is used to propagate errors up the call stack:

```stone
func performOperation() throws {
    let result = try riskyOperation()
    console.log(result)
}
```

The `try?` operator can be used to convert a throwing expression to an optional:

```stone
let result: String? = try? riskyOperation()
```

## 4. Asynchronous Programming

Stone uses the `async` and `await` keywords for handling asynchronous operations.

### Async Functions

Async functions are declared using the `async` keyword:

```stone
async func fetchData() -> String {
    // implementation
}
```

### Await and Error Handling

The `await` keyword signifies a JavaScript Promise. All `await` expressions require an `else` block for error handling, unless used with `try await` or `try? await`.

```stone
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

Stone supports pattern matching using the `match` expression:

```stone
match value {
    case pattern1 -> expression1
    case pattern2 -> expression2
    _ -> defaultExpression
}
```

Example:

```stone
match point {
    case (0, 0) -> "Origin"
    case (x, 0) -> "On x-axis at " + x
    case (0, y) -> "On y-axis at " + y
    case (x, y) -> "At (" + x + ", " + y + ")"
}
```

## 6. Module System

### Module Definition and Exports

In Stone, every file is implicitly a module. By default, all declarations in a file are exported and available to other modules.

Use the `internal` keyword before any declaration to make it private to the module:

```stone
internal let secretKey = "1234567890"
internal func helperFunction() {
    // This function is only available within this module
}
```

### Import Syntax and Mechanisms

Stone uses the `import` keyword to bring declarations from other files into the current scope.

Basic import syntax:

```stone
import { functionName, TypeName } from "filename"
```

The compiler will look for a file named `filename.stone` in the same directory as the current file.

Renaming imports:

```stone
import { originalName: newName } from "filename"
```

This specification provides an overview of the Stone language. It covers the basic syntax, type system, error handling, asynchronous programming, pattern matching, and module system. As the language evolves, this specification will be updated to reflect new features and changes.
