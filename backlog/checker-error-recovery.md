# Checker Error Recovery Analysis

## Problem Statement

When the checker encounters an error, it typically calls `addError()` and returns `nil`. This causes cascading failures where:

1. A function call with errors returns `nil`
2. The variable assignment using that function call fails due to receiving `nil`
3. Further references to that variable also fail since the variable wasn't properly initialized
4. This creates a cascade of error messages for a single root issue

## ✅ COMPLETED IMPLEMENTATIONS

### High Priority Issues (All Completed)

**1. Type Resolution (`resolveType`)** - ✅ FIXED
- **Location**: `checker.go:269-270`
- **Solution**: Returns `&Any{name: "unknown"}` instead of `nil`
- **Impact**: Prevents cascading failures in all type-dependent code

**2. Invalid Float Literals (`checkExpr`)** - ✅ FIXED
- **Location**: `checker.go:1184-1185`
- **Solution**: Returns `&FloatLiteral{Value: 0.0}` instead of `nil`
- **Impact**: Allows expressions using bad floats to continue processing

**3. Interpolated String Chunk Failure (`checkExpr`)** - ✅ FIXED
- **Location**: `checker.go:1200-1202, 1205-1207`
- **Solution**: Replaces failed chunks with `&StrLiteral{"<error>"}` placeholders
- **Impact**: Preserves string structure while handling individual chunk failures

**4. Undefined Variables (`checkExpr`)** - ✅ IMPLEMENTED CRITICAL HALTING
- **Location**: `checker.go:1216-1217`
- **Solution**: Sets `c.halted = true` and returns `nil` to prevent cascading
- **Impact**: Clean error reporting without noise from cascading failures
- **Rationale**: Undefined variables are typically typos/missing declarations - not recoverable

## Current Error Recovery Patterns

### Pattern 1: Direct `addError() → return nil`

**Type Resolution Errors:**
```go
// checker.go:269-270
c.addError(fmt.Sprintf("Unrecognized type: %s", t.GetName()), t.GetLocation())
return nil
```

**Variable Resolution Errors:**
```go
// checker.go:1216-1217 (in checkExpr)
c.addError(fmt.Sprintf("Undefined variable: %s", s.Name), s.GetLocation())
return nil
```

**Invalid Numeric Literals:**
```go
// checker.go:1184-1185 (in checkExpr)
c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
return nil
```

### Pattern 2: Cascading `nil` checks

**Interpolated String Processing:**
```go
// checker.go:1199-1202 (in checkExpr)
cx := c.checkExpr(s.Chunks[i])
if cx == nil {
    return nil  // Cascades error from nested checkExpr
}
```

**Struct Field Type Resolution:**
```go
// checker.go:880-883 (in checkStmt)
fieldType := c.resolveType(field.Type)
if fieldType == nil {
    return nil  // Cascades error from resolveType
}
```

### Pattern 3: `continue` instead of early return
```go
// checker.go:117-118 (import processing)
c.addError(fmt.Sprintf("Failed to resolve import '%s': %v", imp.Path, err), imp.GetLocation())
continue  // Allows processing other imports
```

## Error Recovery Impact Analysis

### High Impact (Causes Significant Cascading)

1. **Type Resolution (`resolveType`)** - Used everywhere, `nil` return breaks all dependent code
2. **Expression Checking (`checkExpr`)** - Core function, `nil` return breaks variable assignments and function calls
3. **Variable Lookups** - `nil` return causes all references to that variable to fail
4. **Function Call Checking** - `nil` return breaks assignments using that function result

### Medium Impact (Some Cascading)

1. **Struct Definition Checking** - Breaks the struct but doesn't affect other definitions
2. **Impl Block Checking** - Breaks method implementations but struct still exists
3. **Trait Implementation** - Breaks trait compliance but struct otherwise works

### Low Impact (Minimal Cascading)

1. **Import Processing** - Uses `continue`, allows other imports to process
2. **Duplicate Field Checking** - Specific validation error, doesn't break type structure
3. **Method Parameter Validation** - Uses `continue` to check other parameters

## Proposed Recovery Strategies

### Strategy 1: Error Recovery Nodes

Create placeholder nodes with appropriate types when errors occur:

```go
// Instead of:
c.addError("Invalid int: " + s.Value, s.GetLocation())
return nil

// Use:
c.addError("Invalid int: " + s.Value, s.GetLocation())
return &IntLiteral{0} // or &ErrorExpression{expectedType: Int}
```

### Strategy 2: Void Type Substitution

For expressions that cannot be recovered, use Void type:

```go
// Instead of:
c.addError("Undefined variable: " + s.Name, s.GetLocation())
return nil

// Use:
c.addError("Undefined variable: " + s.Name, s.GetLocation())
return &Variable{Symbol{Name: s.Name, Type: Void, Mutable: false}}
```

### Strategy 3: Critical Error Halting

Add a `halted` flag to the checker for critical errors:

```go
type checker struct {
    // ... existing fields
    halted bool
}

// For critical errors that make continuation impossible:
c.addError("Critical error message", location)
c.halted = true
return nil
```

## 🚧 REMAINING WORK (Medium Priority)

### Statement-Level Error Recovery

**5. Struct Field Type Resolution Failure** - TODO
- **Location**: `checker.go:880-883` (in `checkStmt`)
- **Current**: Returns `nil` when field type can't be resolved
- **Suggested**: Continue with `Any` type for failed fields

**6. Duplicate Field Names** - TODO
- **Location**: `checker.go:886-887` (in `checkStmt`)
- **Current**: Returns `nil` for entire struct
- **Suggested**: Skip duplicate field, continue with struct processing

**7. Undefined Impl Target** - TODO
- **Location**: `checker.go:898-899` (in `checkStmt`)
- **Current**: Returns `nil` when impl target doesn't exist
- **Suggested**: Could halt (critical) or create placeholder struct

**8. Non-Struct Impl Target** - TODO
- **Location**: `checker.go:904-905` (in `checkStmt`)
- **Current**: Returns `nil` when trying to impl on non-struct
- **Suggested**: Skip impl block, continue processing

### Trait Implementation Error Recovery

**9. Bad Path in Trait Implementation** - TODO
- **Location**: `checker.go:378-379`
- **Current**: Returns `nil` for bad static paths
- **Suggested**: Skip this trait impl, continue with others

**10. Undefined Trait** - TODO
- **Location**: `checker.go:387-388`
- **Current**: Returns `nil` when trait doesn't exist
- **Suggested**: Could halt (critical) or skip trait impl

**11. Not a Trait Type** - TODO
- **Location**: `checker.go:393-394`
- **Current**: Returns `nil` when symbol isn't a trait
- **Suggested**: Skip trait impl, continue processing

**12. Undefined Type for Trait** - TODO
- **Location**: `checker.go:400-401`
- **Current**: Returns `nil` when target type doesn't exist
- **Suggested**: Could halt (critical) or skip trait impl

**13. Not a Struct Type for Trait** - TODO
- **Location**: `checker.go:406-407`
- **Current**: Returns `nil` when target isn't a struct
- **Suggested**: Skip trait impl, continue processing

## Current Strategy (Established)

### ✅ Error Recovery Pattern
For **recoverable errors** (type mismatches, parsing issues):
- Return placeholder objects with reasonable types
- Continue processing to find more errors
- Examples: `&Any{name: "unknown"}`, `&FloatLiteral{Value: 0.0}`, `&StrLiteral{"<error>"}`

### ✅ Critical Error Halting Pattern
For **critical errors** (fundamental missing dependencies):
- Set `c.halted = true`
- Add halted checks to `checkExpr()` and `checkStmt()`
- Return `nil` but prevent further processing
- Examples: Undefined variables

## Implementation Notes

### Completed Infrastructure
- ✅ `halted` flag exists in checker struct
- ✅ Halted checks added to `checkExpr()` and `checkStmt()`
- ✅ Test framework updated for new error patterns

### Remaining Priority
- **Low-Medium**: The remaining items have localized impact
- **Focus**: Can be addressed incrementally when needed
- **Next Phase**: Consider moving to other major features (FFI, etc.)

## Notes

- The integer literal parsing on line 1191-1192 already demonstrates good error recovery - it adds an error but still returns `&IntLiteral{value}` with the parsed value
- Import processing (line 117-118) shows the `continue` pattern works well for iteration contexts
- Some errors might benefit from halting (critical syntax errors) while others need recovery (type mismatches)
