# Generics in Ardlang

## Overview

Ard supports generic types for functions, methods, and structs. Generics use a `$` prefix (e.g., `$T`, `$U`) and can be constrained through explicit type arguments or inferred from usage context.

## Syntax

Generic type parameters begin with `$` in declarations and do not require explicit generic parameter declarations like other languages.

### Functions

```ard
fn map(list: [$A]) [$B] {
  // ...
}
```

When calling a generic function, type arguments can be explicitly provided:

```ard
let floats = map<Int, Float>(ints)
```

The order of type arguments corresponds to the order of generics in the signature.

If type arguments are not provided, the compiler infers them from usage:

```ard
let ints = [1, 2, 3]
let floats: [Float] = map(ints)  // Compiler infers: ints is [Int], result must be [Float]
```

### Structs

Generic structs define type parameters in field declarations:

```ard
struct Box { item: $T }

struct Pair { first: $T, second: $U }
```

When declaring a variable with a generic struct, types can be explicitly specified:

```ard
let box: Box<Int> = Box{ item: 42 }
let pair: Pair<Str, Int> = Pair{ first: "hello", second: 10 }
```

Or inferred from the literal:

```ard
let box = Box{ item: 42 }  // Inferred as Box<Int>
let pair = Pair{ first: "hello", second: 10 }  // Inferred as Pair<Str, Int>
```

### Supported Type Arguments

Generic type parameters accept any valid Ard type:

- **Primitives**: `Int`, `Str`, `Bool`, `Float`
- **Collections**: `[Int]`, `[Str: Int]` (lists and maps)
- **Complex types**: `Maybe<Int>`, `Result<Str, Error>`
- **Function types**: `fn(Int) Int`
- **Nested generics**: `Box<[Int]>`, `Pair<Box<Str>, Int>`

## Implementation

### Type Resolution Pipeline

The compiler resolves generics through a multi-stage pipeline:

1. **Parser**: Parses `<Type1, Type2>` syntax in CustomType nodes via `TypeArgs` field
2. **Checker.resolveType()**: When a CustomType with TypeArgs is encountered, calls `specializeAliasedType()`
3. **specializeAliasedType()**:
   - Calls `collectGenericsFromType()` to extract generic parameter names from the struct definition
   - Validates type argument count matches generic count
   - Replaces each generic (e.g., `$T`) with the provided concrete type
   - Returns a specialized StructDef with updated field types
4. **Variable Declaration Checking**: Compares declared type with inferred type from struct instance

### Type Unification

For generic structs, the instantiation process infers generic parameters:

- When checking a struct literal like `Box{ item: 42 }`, `validateStructInstance()` inspects provided values
- Creates fresh TypeVar instances (internally represented as `Any` types)
- Uses unification to resolve generics from values
- Returns a StructInstance with resolved field types

The variable declaration code then verifies that the declared type (explicit type parameters) matches the inferred type (from literal values).

### Generic Function Calls

When calling a generic function:

1. **Fresh Copy**: Each function call gets its own isolated copy of the function signature with fresh `Any` instances
2. **Mutable Type Resolution**: As each argument is checked, generics are bound in-place within that copy
3. **Single-Pass Checking**: Subsequent arguments see bindings from prior arguments immediately
4. **Safe Mutation**: Mutations only affect the call-site-specific copy, not the original definition

This enables seamless type inference for anonymous function parameters:

```ard
struct User { name: Str }

let users = [User{name: "Alice"}, User{name: "Bob"}]
let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
```

Here, `List::keep` has signature `fn(list: [$T], where: fn($T) Bool) [$T]`. After binding `$T` to `User` from the first argument, the second argument's anonymous function parameter `u` is automatically inferred as `User` without explicit type annotation.

### Generic Type Variables (Any Type)

Internally, the compiler represents generic parameters as `Any` types with mutable state:

```go
type Any struct {
    name   string  // "T", "U", etc.
    actual Type    // The concrete type once resolved
    bound  bool    // Whether actual has been set
}
```

The `bound` flag tracks whether a generic has been resolved. During unification:

- If a generic hasn't been bound, it's resolved immediately: `expectedType.actual = resolvedType; expectedType.bound = true`
- If already bound, the compiler verifies consistency (rejecting conflicting bindings like `T` being both `Int` and `Str`)
- Before type comparisons, a `deref()` helper follows binding chains to ensure the resolved type is visible

### Type Safety

Generic mutations are safe because:

1. Each function **call site** creates a fresh copy of the function signature with fresh `Any` instances
2. These instances are isolated to that call's type checking — not shared with other calls
3. The original function definition's `Any` instances are never mutated
4. Single-threaded type checking prevents race conditions within a call site

The key principle: **copy once per call, then mutate within that copy**. This is safer than mutating a shared definition and simpler than collecting bindings in a map.

## Examples

### Basic Generic Struct

```ard
struct Box { item: $T }

fn main() {
    let int_box: Box<Int> = Box{ item: 42 }
    let str_box: Box<Str> = Box{ item: "hello" }
}
```

### Multiple Generic Parameters

```ard
struct Pair { first: $T, second: $U }

fn main() {
    let numbers: Pair<Int, Int> = Pair{ first: 1, second: 2 }
    let mixed: Pair<Str, Bool> = Pair{ first: "yes", second: true }
}
```

### Nested Generic Types

```ard
struct Container { data: $T }

fn main() {
    let list_container: Container<[Int]> = Container{ data: [1, 2, 3] }
    let map_container: Container<[Str: Int]> = Container{ data: ["a": 1] }
}
```

### Generic Function with Type Inference

```ard
struct User { name: Str }

let users: [User] = [User{name: "Alice"}, User{name: "Bob"}]
let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
```

The parameter `u` is automatically inferred as `User` from the function signature and binding of `$T`.

## Limitations and Future Work

1. **Method bodies with generic field access**: Method implementations cannot directly access generic fields (e.g., `@field`), as TypeVars are unbound at definition time. Methods can use generics only in signatures.

2. **Non-struct generics**: Currently only supports generic structs. Other generic types (enums, functions, etc.) would need similar implementation.

3. **Generic bounds/constraints**: No support for trait bounds or type constraints yet.

## Testing

Comprehensive test coverage includes:

- Basic single-parameter generics for functions and structs
- Multiple generic parameters
- Nested generic types (Box<[Int]>, Container<[Str: Int]>)
- Type argument count validation
- Type mismatch detection
- Anonymous function parameter inference in generic functions
- All integration tests pass with no regressions

## How It Works: Checker Algorithm

The core algorithm uses the `Type.equal()` comparator:

- Usually called as `actual.equal(expected)`
- Allows `actual` to "refine" `expected` when `expected` is a generic type
- Once a generic is refined, it is closed to matching other types

In functions and methods:

1. Open generics are collected from the signature
2. If type arguments are provided, generics are refined with them
3. Generic parameters are refined through inference of provided values
4. Type checking validates argument types against the refined signature
