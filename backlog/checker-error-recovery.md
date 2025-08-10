# Checker Error Recovery Analysis

## Problem Statement

When the checker encounters an error, it typically calls `addError()` and returns `nil`. This causes cascading failures where:

1. A function call with errors returns `nil`
2. The variable assignment using that function call fails due to receiving `nil`  
3. Further references to that variable also fail since the variable wasn't properly initialized
4. This creates a cascade of error messages for a single root issue

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

## Locations Requiring Updates

### `checker.go` Functions to Update:

1. **`resolveType()`** (line 269-270) - HIGH PRIORITY
   - Return a placeholder type instead of nil
   - Consider using Unknown type or Void

2. **`checkExpr()`** multiple locations - HIGH PRIORITY  
   - Line 1184-1185: Invalid float literals
   - Line 1191-1192: Invalid int literals (note: already returns IntLiteral!)
   - Line 1200-1202: Interpolated string chunk failure 
   - Line 1205-1207: ToString trait mismatch
   - Line 1216-1217: Undefined variable

3. **`checkStmt()`** multiple locations - MEDIUM PRIORITY
   - Line 880-883: Field type resolution failure
   - Line 886-887: Duplicate field names
   - Line 898-899: Undefined impl target
   - Line 904-905: Non-struct impl target

4. **Trait Implementation Functions** - MEDIUM PRIORITY
   - Lines 378-379, 387-388, 393-394, 400-401, 406-407
   - These could potentially continue with partial implementations

### Other Files:
- `types.go` - Various type-related `return nil` statements
- `std_lib.go` - Standard library resolution failures

## Implementation Priority

1. **Phase 1**: Fix type resolution and expression checking (high cascading impact)
2. **Phase 2**: Fix statement checking and struct processing  
3. **Phase 3**: Add critical error halting mechanism
4. **Phase 4**: Optimize error recovery for better user experience

## Notes

- The integer literal parsing on line 1191-1192 already demonstrates good error recovery - it adds an error but still returns `&IntLiteral{value}` with the parsed value
- Import processing (line 117-118) shows the `continue` pattern works well for iteration contexts
- Some errors might benefit from halting (critical syntax errors) while others need recovery (type mismatches)
