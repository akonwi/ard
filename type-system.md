# Ard Language Type System Design

This document explores different approaches to implementing generics and type specialization in the Ard language type system.

## Approaches

### 1. Type Inference System

A comprehensive type inference system that propagates type information through the AST and infers concrete types for generics.

**Core Concepts:**
- Type variables represent unknown types that need to be inferred
- Unification algorithm to reconcile different constraints on the same type variable
- Type propagation through expressions, statements, and function calls
- Bidirectional type checking (combining bottom-up and top-down approaches)

**Implementation Considerations:**
- Would require tracking constraints and relations between type variables
- Need to handle recursive types and avoid infinite loops in inference
- Requires changes to the fundamental design of the checker
- More complex but provides the most flexibility

**Type Propagation Mechanics:**

Type propagation happens in multiple directions:

**Constraint Graph Pseudocode Example:**

Here's how a constraint graph for type inference might work:

```
// Type variable definition
struct TypeVar {
  id: String               // Unique identifier
  constraints: [Constraint] // Constraints on this type variable
  solution: Type?          // The concrete type, if solved
}

// Constraint types
enum Constraint {
  Equality(TypeVar, Type)   // tv = type
  Compatibility(TypeVar, TypeVar) // tv1 ~ tv2
  Instance(TypeVar, TypeVar)      // tv1 is instance of tv2
  Field(TypeVar, String, TypeVar) // tv1.field = tv2
}

// Example: Processing the expression `let x = maybe.some("hello")`

// 1. Create type variables
α = TypeVar(id: "α")        // Type of "hello"
β = TypeVar(id: "β")        // Type of some's input parameter
γ = TypeVar(id: "γ")        // Type inside Maybe<T>
δ = TypeVar(id: "δ")        // Type of the overall expression

// 2. Add constraints from the literal
addConstraint(Equality(α, StringType))

// 3. Add constraints from the function type
// maybe.some is represented as fn<T>(T) -> Maybe<T>
addConstraint(Compatibility(α, β))  // Argument must be compatible with parameter
addConstraint(Instance(γ, β))       // γ is the same type as β (the type parameter T)

// 4. Add constraints for the return type
// The return type is Maybe<T> where T is γ
returnType = MaybeType(innerType: γ)
addConstraint(Equality(δ, returnType))

// 5. Now solve the constraint graph:
function solveConstraints() {
  // Iteratively process constraints until fixed point is reached
  repeat until no changes:
    for each constraint in constraints:
      switch constraint {
        case Equality(tv, type):
          if tv.solution is null:
            tv.solution = type
          else if tv.solution != type:
            reportError("Type mismatch")
          
        case Compatibility(tv1, tv2):
          if tv1.solution is not null and tv2.solution is null:
            tv2.solution = tv1.solution
          else if tv2.solution is not null and tv1.solution is null:
            tv1.solution = tv2.solution
          else if tv1.solution is not null and tv2.solution is not null:
            if not compatible(tv1.solution, tv2.solution):
              reportError("Incompatible types")
              
        // Process other constraint types similarly
      }
}

// 6. After solving, we have:
// α = StringType (from direct constraint)
// β = StringType (from compatibility with α)
// γ = StringType (from Instance constraint with β)
// δ = Maybe<StringType> (from equality with MaybeType(γ))

// 7. Therefore, x is inferred to be of type Maybe<StringType>, or Str?
```

For a more complex example with maybe.none():

```
// Processing: let x: Str? = maybe.none()

// 1. Create type variables
α = TypeVar(id: "α")  // Type inside Maybe<T> returned by none()
β = TypeVar(id: "β")  // Type of the overall expression maybe.none()

// 2. Add constraints from the function type
// maybe.none is represented as fn() -> Maybe<T>
// where T is unconstrained
returnType = MaybeType(innerType: α)
addConstraint(Equality(β, returnType))

// 3. Add contextual constraint from the variable declaration
expectedType = MaybeType(innerType: StringType)
addConstraint(Equality(β, expectedType))

// 4. After solving, we have:
// α = StringType (from contextual constraint)
// β = Maybe<StringType> (from function return type)

// 5. Therefore, maybe.none() gets specialized to return Str?
```

1. **Bottom-up Propagation:**
   - Types flow from subexpressions to containing expressions
   - Example: In `a + b`, the types of `a` and `b` determine the type of the expression
   - Function arguments propagate constraints to type parameters

2. **Top-down Propagation:**
   - Expected types flow from context to expressions
   - Example: In `let x: Str? = expr`, the type `Str?` constrains what `expr` can be
   - Return type declarations constrain the body of functions

3. **Constraint Collection and Solving:**
   - Each expression generates constraints on type variables
   - The type checker builds a constraint graph connecting type variables
   - A unification algorithm solves for all variables simultaneously
   - Example: 
     ```
     let x = maybe.some(y)
     ```
     1. Assign type variables: `some` is `fn<T>(T) -> Maybe<T>`, result is `Maybe<α>`
     2. Generate constraint: `typeof(y) = α`
     3. If `y` is a string, propagate: `α = Str`
     4. Resolve `x` as `Maybe<Str>`

4. **Type Environment and Scoping:**
   - The type checker maintains an environment mapping variables to their types
   - Type variables are created within specific scopes
   - When leaving a scope, remaining type variables may be either:
     - Defaulted to Any (if unresolved)
     - Result in a type error (if insufficient constraints)

5. **Handling Generic Functions:**
   - Generic functions receive fresh type variables for each call
   - Example: Two calls to `maybe.some()` with different argument types
   - Type parameters are unified with the concrete types of arguments
   - Return types are specialized based on resolved type parameters

**Examples:**
```ard
// With type inference:
let x = maybe.some("hello")  // x inferred as Str?
let y: Int? = maybe.none()   // Type parameter inferred from context
```

**Advantages:**
- Most flexible and powerful approach
- Supports complex generic type scenarios
- Provides good error messages by tracking constraint origins
- Works well with polymorphic functions and containers

**Challenges:**
- Most complex to implement correctly
- May require significant refactoring of existing code
- Need to handle constraint solving and type variable substitution
- More difficult to debug type errors

### 2. Type Parameter Binding with '$' Notation

This approach uses a leading '$' to implicitly declare generic type parameters, providing a clean and minimalist syntax for Ard.

**Core Concepts:**
- Type parameters identified by a leading '$' in the name (e.g., `$T`)
- Implicit declaration through usage in function signatures or type definitions
- Type inference from provided values where possible
- Explicit type annotations required when inference is insufficient

**Implementation Considerations:**
- The '$' prefix clearly distinguishes type parameters from concrete types
- Type parameters can be referenced multiple times to enforce type consistency
- No need for separate type parameter declaration blocks
- Type checking focuses on unification and consistency of generic types

**Implementation Details:**

1. **Parser Changes:**
   - Update the AST to recognize and handle '$'-prefixed identifiers as type parameters
   - Extend the type parser to handle generic type references
   - Track the scope of type parameters within function and type definitions

2. **Type Checking Process:**
   - Create a mapping of generic parameter names to type variables during type checking
   - When a generic parameter is first encountered, create a fresh type variable for it
   - When the same parameter is referenced again, use the same type variable
   - Apply unification to ensure consistent use of type parameters

3. **Handling Maybe Types:**
   - Special case for `maybe.none()` to look at contextual type information
   - For assignment like `let x: $T? = maybe.none()`, extract '$T' and use it to type the expression
   - For function arguments like `fn(x: $T?)`, extract '$T' when calling with `maybe.none()`

4. **Generic Type Resolution Algorithm:**
   ```
   function resolveGenericFunction(fn, args, expectedReturnType):
     // Create type variables for each generic parameter
     typeVars = {}
     
     // Process parameter types
     for i in range(fn.params.length):
       paramType = fn.params[i].type
       argType = args[i].type
       
       if isGenericType(paramType):
         // Extract generic parameters used in this type
         genericParams = extractGenericParams(paramType)
         
         for param in genericParams:
           if param not in typeVars:
             typeVars[param] = fresh TypeVariable()
           
         // Substitute type variables into the parameter type
         concreteParamType = substitute(paramType, typeVars)
         
         // Unify with argument type
         unify(concreteParamType, argType)
     
     // Resolve return type
     if isGenericType(fn.returnType):
       genericParams = extractGenericParams(fn.returnType)
       
       for param in genericParams:
         if param not in typeVars:
           // If we have expected return type, try to extract info from it
           if expectedReturnType != null:
             typeVars[param] = inferFromExpectedType(param, fn.returnType, expectedReturnType)
           else:
             typeVars[param] = fresh TypeVariable()
             
       returnType = substitute(fn.returnType, typeVars)
     else:
       returnType = fn.returnType
     
     return returnType
   ```$' prefix clearly distinguishes type parameters from concrete types
- Type parameters can be referenced multiple times to enforce type consistency
- No need for separate type parameter declaration blocks
- Type checking focuses on unification and consistency of generic types

**Implementation Details:**

1. **Parser Changes:**
   - Update the AST to recognize and handle '$'-prefixed identifiers as type parameters
   - Extend the type parser to handle generic type references
   - Track the scope of type parameters within function and type definitions

2. **Type Checking Process:**
   - Create a mapping of generic parameter names to type variables during type checking
   - When a generic parameter is first encountered, create a fresh type variable for it
   - When the same parameter is referenced again, use the same type variable
   - Apply unification to ensure consistent use of type parameters

3. **Handling Maybe Types:**
   - Special case for `maybe.none()` to look at contextual type information
   - For assignment like `let x: $T? = maybe.none()`, extract '$T' and use it to type the expression
   - For function arguments like `fn(x: $T?)`, extract '$T' when calling with `maybe.none()`

4. **Generic Type Resolution Algorithm:**
   ```
   function resolveGenericFunction(fn, args, expectedReturnType):
     // Create type variables for each generic parameter
     typeVars = {}
     
     // Process parameter types
     for i in range(fn.params.length):
       paramType = fn.params[i].type
       argType = args[i].type
       
       if isGenericType(paramType):
         // Extract generic parameters used in this type
         genericParams = extractGenericParams(paramType)
         
         for param in genericParams:
           if param not in typeVars:
             typeVars[param] = fresh TypeVariable()
           
         // Substitute type variables into the parameter type
         concreteParamType = substitute(paramType, typeVars)
         
         // Unify with argument type
         unify(concreteParamType, argType)
     
     // Resolve return type
     if isGenericType(fn.returnType):
       genericParams = extractGenericParams(fn.returnType)
       
       for param in genericParams:
         if param not in typeVars:
           // If we have expected return type, try to extract info from it
           if expectedReturnType != null:
             typeVars[param] = inferFromExpectedType(param, fn.returnType, expectedReturnType)
           else:
             typeVars[param] = fresh TypeVariable()
             
       returnType = substitute(fn.returnType, typeVars)
     else:
       returnType = fn.returnType
     
     return returnType
   ```

**Examples:**
```ard
// Generic function with type parameter $T
fn identity(value: $T) $T {
  value
}

// Type parameter inferred from argument
let s = identity("hello")  // $T = Str
let n = identity(42)       // $T = Int

// Multiple type parameters
fn convert(value: $T, converter: ($T) -> $U) $U {
  converter(value)
}

// Generic collection type
type Box {
  value: $T
}

// Implementation with same type parameter
impl (b: Box) {
  fn get() $T {
    b.value
  }
  
  fn map(fn_map: ($T) -> $U) Box {
    Box { value: fn_map(b.value) }
  }
}
```

**Usage Examples Aligned with Ard's Philosophy:**

```ard
// Generic math function
fn add(a: $T, b: $T) $T {
  a + b  // Works for any type supporting addition
}

// Generic container operations
fn map(array: [$T], mapper: ($T) -> $U) [$U] {
  mut result: [$U] = []
  for item in array {
    result.push(mapper(item))
  }
  result
}

// Using Maybe types
fn find(items: [$T], predicate: ($T) -> Bool) $T? {
  for item in items {
    if predicate(item) {
      return maybe.some(item)
    }
  }
  maybe.none()  // Return type inferred from context
}
```

**Integration with Existing Features:**

1. **With Maybe types:**
```ard
// Type inference with context
fn get_first(items: [$T]) $T? {
  if items.size() > 0 {
    maybe.some(items[0])
  } else {
    maybe.none()  // Type inferred from return annotation
  }
}

// Explicit type required when context insufficient
let empty: Str? = maybe.none()
```

2. **With type unions:**
```ard
// Generic union handling
type Either {
  left: $T
  right: $U
}

fn process(e: Either) Str {
  match e {
    { left: $T } => "Left: " + it.left.to_str(),
    { right: $U } => "Right: " + it.right.to_str()
  }
}
```

**Advantages:**
- Clean, intuitive syntax that fits Ard's minimalist design
- No extra declaration syntax needed
- Natural to read and write
- Type parameters clearly distinguished by '$' prefix
- Works well with type inference

**Challenges:**
- Managing scope of generic parameters across complex expressions
- Providing clear error messages for incorrectly matched generic parameters
- Handling cases where type inference is ambiguous
- Ensuring type safety without being overly restrictive

### 3. Runtime Type Resolution

This approach defers some type checking to runtime, allowing for more dynamic behavior while maintaining safety guarantees.

**Core Concepts:**
- Static type checking for basic compatibility
- Runtime type information carried with values
- Dynamic type checks at critical points (like casts or generic instantiations)
- Type erasure vs. reification tradeoffs

**Implementation Considerations:**
- Runtime type representation needs to be lightweight and efficient
- Type checking split between compile-time and runtime phases
- Error handling needs to account for runtime type errors
- Impact on performance and code size

**Examples:**
```ard
// Static checking with runtime fallback
fn process<T>(value: Any) T {
  // Runtime check that value is of type T
  if value is T {
    return value as T
  } else {
    error("Type mismatch: expected " + T.name)
  }
}

// Using runtime type information
let values: [Any] = [1, "hello", true]
for value in values {
  match value {
    Int => io.print("Got an integer: " + value.to_str()),
    Str => io.print("Got a string: " + value),
    _ => io.print("Got something else")
  }
}
```

**Advantages:**
- More flexibility for heterogeneous collections and dynamic behavior
- Can handle cases where static typing is too restrictive
- Easier migration path from dynamic to static typing
- Simplifies some generic programming patterns

**Challenges:**
- Runtime overhead for type checking
- Some errors only detected at runtime
- More complex runtime representation
- Hard to optimize effectively

### 4. Type Substitution

This approach creates specialized versions of generic functions on-demand during type checking based on concrete type arguments.

**Core Concepts:**
- Generic functions/types serve as templates
- Type checker instantiates specialized versions for each unique set of type arguments
- Monomorphization to create type-specific implementations
- Similar to how C++ templates and Rust generics work

**Implementation Considerations:**
- Need to manage creation and caching of specialized versions
- Type checking happens on the specialized versions
- Code generation may duplicate function implementations
- Handle recursive types and unbounded instantiation

**Examples:**
```ard
// Generic function definition
fn map<T, U>(array: [T], fn: (T) -> U) [U] {
  // implementation
}

// When used with specific types:
let numbers = [1, 2, 3]
let strings = map(numbers, (n) => n.to_str())

// The type checker creates a specialized version:
// fn map_int_str(array: [Int], fn: (Int) -> Str) [Str]
```

**Advantages:**
- High performance at runtime (no dynamic dispatch or type information)
- Complete type safety at compile time
- Can optimize each specialized version
- Type errors are caught early

**Challenges:**
- Can lead to code bloat with many specializations
- Harder to implement separate compilation
- May require complex constraint solving for advanced cases
- Can increase compile times significantly

### 5. Contextual Types

This approach uses the expected type from surrounding context to guide type inference and specialization.

**Core Concepts:**
- Expected types flow down from usage context to expressions
- Type inference driven by how values are used
- Bidirectional type checking with propagation of expected types
- Context-dependent type specialization

**Implementation Considerations:**
- Track expected types during type checking
- Combine expected type with inferred type for better type resolution
- Handle ambiguous cases where context doesn't provide enough information
- Balance between flexibility and predictability

**Examples:**
```ard
// Type inferred from assignment target
let x: Str? = maybe.none()   // none() inferred to return Str?

// Type inferred from function argument position
fn process(value: Int?) { /* ... */ }
process(maybe.none())       // none() inferred to return Int?

// Type inferred from return value
fn getData() Str? {
  maybe.none()              // none() inferred to return Str?
}
```

**Advantages:**
- More concise code (fewer type annotations needed)
- Better inference with less ambiguity
- Feels more natural to programmers
- Works well with function overloading

**Challenges:**
- Complex interactions with other type system features
- May create subtle behavior differences depending on context
- Can be harder to predict/understand for developers
- Error messages might be less precise