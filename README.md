# Ard Lang

## What's Ard?
 - __A__ __R__-eally __D__-ope Language
 - [ard is slang for “alright.”](https://www.dictionary.com/e/slang/ard/)
 - [Irish and Scottish Gaelic word meaning 'high, lofty', 'above the ground, elevated](https://www.oxfordreference.com/display/10.1093/oi/authority.20110803095422661)
 - Ardbeg is my favorite scotch and I was drinking it when I came up with this name

## Language Description

Ard is a modern, programming language designed for legibility, simplicity, and type-safety.
It combines elements from JavaScript, Swift, Go, and Rust.

## Goals

- **Readability**: Ard code should be easy to read and understand.
- **Simple**: There should be one obvious way to do things.
- **Safety**: Type errors are caught at compile time and runtime errors must be handled.
- **Reliable**: Built on Go's runtime, so programs can be fast and efficient.

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
- A Return type is specified after the parameter list
  - In order to declare the function as non-returning, omit the return type
- There is no `return` keyword. The last expression is the returned value

```ard
fn greet(name: Str) Str {
  "Hello, {name}!"
}
```

#### Named Arguments

Functions can be called with named arguments allowing them to be in any order.

```ard
fn greet(name: Str, age: Int) Str {
  "Hello, {name}! You are {age.to_str()} years old."
}

// positional arguments (order matters)
greet("Alice", 25)

// named arguments (order doesn't matter)
greet(age: 30, name: "Bob")
```

When using named arguments, all arguments must be named. Mixing positional arguments with named arguments is not supported.

Functions are first class and can therefore be used as arguments

```ard
fn map(list: [Int], do: fn(Int) Int) [Int] {
  let mapped: [Int] = []
  for i in list {
    mapped.push(do(i))
  }
  mapped
}

map([1,2,3], fn(i: Int) Int { i*2 })
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

for key, val in map {
  io::print("key: {key} = value({val})")
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

// matching on integers
let grade = match score {
  0..59 => "F"
  60..69 => "D"
  70..79 => "C"
  80..89 => "B"
  90..100 => "A"
  _ => "Invalid score"
}

// mixing specific values and ranges
let message = match value {
  0 => "zero"
  1..10 => "small number"
  42 => "the answer"
  100..1000 => "big number"
  _ => "something else"
}
```

### Iteration

C style for loop:

```ard
for mut i = 0; i <= 5; i =+1 {
  io::print(i.to_str())
}
```

Numeric inclusive range:
```ard
for i in 1..10 {
	io::print(i) // 1, 2, 3, 4, 5, 6, 7, 8, 9, 10
}
```

Iterating over a list:
```ard
let fruits = ["apple", "banana", "cherry"]
for fruit in fruits {
  io::print(fruit)
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

Structs can have methods. Use an `impl` block to define methods on a struct.
Within a method, its properties can be accessed with the `@` prefix

```ard
impl Person {
  fn get_intro() Str {
    "My name is {@name}"
  }

  fn greet(other: Person) Str {
    "Hello, {other.name}"
  }
}

person.get_intro() // "My name is Alton"
```

Static functions can be scoped to a struct by declaring the function with a name prefixed with the struct name:

```ard
struct Todo {
  title: Str,
  completed: Bool
}

// a helper function for a constructor
fn Todo::new(title: Str) Todo {
  Todo { title: title, completed: false }
}

let first = Todo::new("clean")
```

### Enums

Enums are used to enumerate a discrete set of values.
They are simply labeled integers and cannot have associated values.

```ard
enum Status {
  active,
  inactive,
  pending
}
```

The static operator (`::`) is used to access variants.
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

### Nullable Values

To declare a type that could be present or not, add `?` to the end. This declares it as a built-in `Maybe` type. Values can be created with the `ard/maybe` package from the standard library.
A maybe type can either have a value (`some`) or be empty (`none`).

```ard
use ard/maybe

mut maybe_name: Str? = maybe::none()
maybe_name = maybe::some("Joe")

match maybe_name {
  n => "Hello, {n}",
  _ => "Hello, stranger"
}
```

Maybe types have `is_some()` and `is_none()` methods to peek at presence without consuming the value.
To access the value, without the need for a `match` expression, use the `or(default: $V)` method, which returns the value if it is present, or the provided default value.

```ard
let maybe_name: Str? = maybe::some("Joe")
let name: Str = maybe_name.or("Anonymous")
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
    Str => io::print("String: {it}"),
    Int => io::print("Number: {it.to_str()}")
  }
}
```

### Pattern Matching

Ard supports powerful pattern matching with the `match` expression. Different types support different patterns:

#### Integer Matching

Match on specific integer values or ranges:

```ard
let category = match age {
  0..12 => "child"
  13..19 => "teenager"
  21 => "legal drinking age"
  65..120 => "senior"
  _ => "adult"
}
```

Integer ranges are inclusive (`1..10` includes both 1 and 10). When patterns overlap, the first match wins.

#### Other Pattern Types

- **Booleans**: Match on `true` or `false`
- **Enums**: Match on enum variants like `Status::active`
- **Maybe types**: Match on value binding or `_` for none
- **Type unions**: Match on type with `it` binding
- **Results**: Match on `ok(value)` or `err` patterns

### Qualified static paths
A static path is a sequence of `name::thing`. Ard has a preference for simple paths where possible, i.e. only one `::`.
In order to reach further into a package for something, make that import explicit with `use name/thing/nested`,
so that deeply nested access can be simply called with `nested::thing`.

### Errors
Ard does not have exceptions. Instead, errors are represented as values. The built-in `Result<$Val, $Err>` type can be used as a special type union of a success value and an error value.

#### Result Declaration Sugar
For convenience, `Result<T, E>` can be written using the sugar syntax `T!E`. Both forms are equivalent:

```ard
// These are equivalent:
fn divide_verbose(a: Int, b: Int) Result<Int, Str> { ... }
fn divide_concise(a: Int, b: Int) Int!Str { ... }
```

```ard
fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("Cannot divide by zero"),
    false => Result::ok(a/b),
  }
}
```

Similar to type unions, results can be matched to control conditional execution.

```ard
match divide(42, 0) {
  ok(num) => io::print(num.to_str()),
  err => io::print(err),
}
```

The only way to ignore errors is to use the `.or()` method to provide a default value if the result is not ok.

```ard
let num = divide(a, b).or(0)
io::print("got {num.to_str()})
```

Another alternative to ignoring the error is to propagate it to callers. This can be achieved with the `try` keyword.

```ard
// attempt at (a / b) + 10
fn do_math(a Int, b Int) Int!Str {
  let num = try divide(a, b)
  Result::ok(num + 10)
}
```

The `try` keyword will unwrap the result and if the result is an error, it will act as an early return to pass on the failure result.

Note: `try` can only be used in function blocks

### Modules
See the docs in [modules](./modules)

### Async Programs
See the docs in [async](./async)
