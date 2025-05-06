# Maybe and Generic Type Implementation for Ard

This document outlines the specific implementation details for adding support for Maybe types and generic types (using `$T` syntax) in the Ard language.

## Maybe Type Implementation

```go
// In checker/types.go
type Maybe struct {
  of Type  // The inner type
}

func (m *Maybe) String() string {
  return m.of.String() + "?"
}

func (m *Maybe) get(name string) Type {
  // Methods like .or()
  switch name {
  case "or":
    return &FunctionDef{
      Name: "or",
      Parameters: []Parameter{{Name: "default", Type: m.of}},
      ReturnType: m.of,
    }
  default:
    return nil
  }
}

func (m *Maybe) equal(other Type) bool {
  // Maybe<T> equals Maybe<U> if T equals U
  if otherMaybe, ok := other.(*Maybe); ok {
    return m.of.equal(otherMaybe.of)
  }
  return false
}
```

## Generic Type Support

```go
// A generic type parameter like $T
type GenericType struct {
  name string      // The name without '$' prefix
  bound Type       // Optional bound for the type (for future constraints)
  solution Type    // The concrete type solution, if assigned
}

func (g *GenericType) String() string {
  if g.solution != nil {
    return g.solution.String()
  }
  return "$" + g.name
}

func (g *GenericType) get(name string) Type {
  if g.solution != nil {
    return g.solution.get(name)
  }
  return nil // Cannot access properties of unresolved generic type
}

func (g *GenericType) equal(other Type) bool {
  if g.solution != nil {
    return g.solution.equal(other)
  }

  // An unresolved generic type can be equal to any concrete type
  // This allows for initial binding during type checking
  return true
}
```

## Enhancements to the Type Checker

```go
// In checker/checker.go

// Add a map to track generic parameters in scope
type checker struct {
  // Existing fields
  genericParams map[string]*GenericType // Track generic parameters
}

// When checking a function with generic parameters:
func (c *checker) checkFunction(fn *ast.FunctionDeclaration) *FunctionDef {
  // Create a new scope for generic parameters
  oldGenericParams := c.genericParams
  c.genericParams = make(map[string]*GenericType)

  // Process parameter types and look for generic parameters ($T, etc.)
  for _, param := range fn.Parameters {
    c.collectGenericParams(param.Type)
  }

  // Process return type for generic parameters
  c.collectGenericParams(fn.ReturnType)

  // Check the function body with generic parameters in scope
  // ...

  // Restore previous generic parameter scope
  c.genericParams = oldGenericParams

  // Return function definition
}

// Helper to collect generic parameters from a type
func (c *checker) collectGenericParams(t ast.Type) {
  // If type name starts with $, it's a generic parameter
  if t.Name[0] == '$' {
    name := t.Name[1:] // Remove $ prefix
    if _, exists := c.genericParams[name]; !exists {
      c.genericParams[name] = &GenericType{name: name}
    }
  }

  // Recursively check nested types (like in Maybe<$T>)
  // ...
}

// Modify type resolution to handle generic types
func (c *checker) resolveType(astType ast.DeclaredType) Type {
  // If it's a generic type parameter
  if astType.GetName()[0] == '$' {
    name := astType.GetName()[1:] // Remove $ prefix
    if genType, exists := c.genericParams[name]; exists {
      return genType
    }
    // Error - generic type not in scope
    return nil
  }

  // Handle regular types as before
  // ...
}
```

## Standard Library Maybe Functions

```go
// In checker/std_lib.go

// Update package functions for Maybe
func getInMaybe(name string) symbol {
  switch name {
  case "none":
    // Create a generic type parameter for the Maybe inner type
    return &FunctionDef{
      Name: "none",
      Parameters: []Parameter{},
      ReturnType: &Maybe{of: &GenericType{name: "T"}},
    }

  case "some":
    // Create a generic type that gets bound to the argument type
    genericT := &GenericType{name: "T"}
    return &FunctionDef{
      Name: "some",
      Parameters: []Parameter{{Name: "value", Type: genericT}},
      ReturnType: &Maybe{of: genericT}, // Use the same generic type
    }

  default:
    return nil
  }
}
```

## Type Checking for Generic Functions

```go
// Type checking a call to a generic function
func (c *checker) checkFunctionCall(call *ast.FunctionCall, fn *FunctionDef) Expression {
  // For each argument, check its type
  argExprs := make([]Expression, len(call.Args))
  for i, arg := range call.Args {
    argExprs[i] = c.checkExpr(arg)
  }

  // If function has generic parameters, create a specialized version
  if hasGenericParams(fn) {
    // Create a mapping of generic parameters to concrete types
    typeMap := make(map[string]Type)

    // Infer types from arguments
    for i, param := range fn.Parameters {
      if genType, ok := param.Type.(*GenericType); ok {
        if existing, exists := typeMap[genType.name]; exists {
          // Ensure consistent types for the same generic parameter
          if !existing.equal(argExprs[i].Type()) {
            c.addError(fmt.Sprintf("Type mismatch for generic parameter $%s", genType.name))
            return nil
          }
        } else {
          // Bind the generic parameter to the argument type
          typeMap[genType.name] = argExprs[i].Type()
        }
      }
    }

    // Create specialized function with generic parameters substituted
    specializedFn := specializeFn(fn, typeMap)

    // Return function call with specialized function
    return &FunctionCall{
      Name: call.Name,
      Args: argExprs,
      fn: specializedFn,
    }
  }

  // Normal function call processing
  // ...
}

// Substitute generic parameters with concrete types
func specializeFn(fn *FunctionDef, typeMap map[string]Type) *FunctionDef {
  specialized := &FunctionDef{
    Name: fn.Name,
    Parameters: make([]Parameter, len(fn.Parameters)),
    ReturnType: substituteType(fn.ReturnType, typeMap),
    Body: fn.Body, // Body remains the same
  }

  // Substitute types in parameters
  for i, param := range fn.Parameters {
    specialized.Parameters[i] = Parameter{
      Name: param.Name,
      Type: substituteType(param.Type, typeMap),
      Mutable: param.Mutable,
    }
  }

  return specialized
}

// Substitute generic parameters in a type
func substituteType(t Type, typeMap map[string]Type) Type {
  switch typ := t.(type) {
  case *GenericType:
    if concrete, exists := typeMap[typ.name]; exists {
      return concrete
    }
    return typ
  case *Maybe:
    return &Maybe{of: substituteType(typ.of, typeMap)}
  // Handle other compound types
  default:
    return t
  }
}
```

## Handling maybe.none() with Context

```go
// Special case handling for maybe.none()
func (c *checker) checkMaybeNone(call *ast.FunctionCall, expectedType Type) Expression {
  // If we have an expected type that's a Maybe
  if expectedMaybe, ok := expectedType.(*Maybe); ok {
    // Create a specialized version of none() with the expected inner type
    specializedNone := &FunctionDef{
      Name: "none",
      Parameters: []Parameter{},
      ReturnType: &Maybe{of: expectedMaybe.of},
    }

    return &FunctionCall{
      Name: "none",
      Args: []Expression{},
      fn: specializedNone,
    }
  }

  // If no context, return a generic none
  return &FunctionCall{
    Name: "none",
    Args: []Expression{},
    fn: &FunctionDef{
      Name: "none",
      Parameters: []Parameter{},
      ReturnType: &Maybe{of: &GenericType{name: "T"}},
    },
  }
}
```
