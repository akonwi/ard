# List Literal Function Type Inference Issue - ✅ RESOLVED

## Summary

The type checker failed to properly infer types for list literals containing function values, resulting in runtime panics due to nil type pointers in `ListLiteral.Type()`.

**FIXED**: Changed pointer comparison (`!=`) to structural comparison (`.equal()`) in `checkList()`.

## Problem Description (Historical)

When creating list literals that contain function values (closures, decoders, etc.), the type checker cannot unify the element types, leading to the `_type` field being left as `nil` in the `ListLiteral` AST node.

### Failing Examples

```ard
// This fails during type checking
let decoders = [decode::string, decode::string]  // panic: nil pointer dereference

// This also fails
let str_dec1 = decode::string
let str_dec2 = decode::string
let decoders = [str_dec1, str_dec2]  // panic: nil pointer dereference
```

### Working Examples

```ard
// This works fine - single element
let single_decoder = [decode::string]  // Works

// This works - non-function types
let numbers = [1, 2, 3]  // Works fine
let strings = ["a", "b", "c"]  // Works fine
```

## Technical Details

### Stack Trace Location
- **File**: `checker/nodes.go:91`
- **Function**: `ListLiteral.Type()`
- **Issue**: `l._type` is `nil` when accessed

### Root Cause Analysis

1. **Type Unification Problem**: The type checker's unification algorithm in `checker.go` cannot properly unify function types, especially when they have complex generic signatures like decoders.

2. **Missing Type Assignment**: During expression checking in `checker.checkExpr()`, the `ListLiteral._type` field is not being set when element type unification fails.

3. **Function Type Complexity**: Decoder functions have signatures like `fn(Dynamic) T![Error]` with generics, making type unification more complex than simple primitive types.

## Impact

- **Immediate**: Prevents implementation of compositional patterns like `one_of([decoder1, decoder2])`
- **Broader**: Limits any use case requiring lists of function values
- **Workaround**: Currently requires single-element lists or avoiding direct list literals with functions

## Current Workaround

For the `one_of` decoder implementation, we're limited to:

```ard
// Works - single decoder
let decoder = decode::one_of([decode::string])

// Fails - multiple decoders  
// let decoder = decode::one_of([decode::string, decode::int])
```

## Proposed Solutions

### Option 1: Enhanced List Literal Type Inference
- Improve the type unification algorithm in `checker.checkExpr()` for `ListLiteral`
- Add special handling for function types with generic parameters
- Ensure `_type` is always set, even if to a more general type

### Option 2: Explicit Type Annotations
- Allow explicit type annotations for list literals:
  ```ard
  let decoders: [Decoder<Str>] = [decode::string, custom_string_decoder]
  ```

### Option 3: Alternative Syntax
- Introduce a different syntax for function collections:
  ```ard
  let decoder = decode::one_of(decode::string | custom_string_decoder)
  ```

## Resolution - ✅ COMPLETED

**Root Cause**: In `checker/checker.go:989`, the type comparison used pointer equality (`!=`) instead of structural equality.

**Fix**: Changed line 989 from:
```go
} else if elementType != element.Type() {
```
to:
```go  
} else if !elementType.equal(element.Type()) {
```

This aligns with all other type comparisons in the codebase, which use `.equal()` for structural equality.

**Impact**: All decoder compositional patterns now work correctly, including:
- `decode::one_of([decode::string, custom_decoder])`
- Lists of any function types with identical signatures
- Complex compositional decoder usage

## Priority

**✅ RESOLVED** - No longer impacts decoder patterns or functional programming constructs.

## Related Code

- `checker/nodes.go:90-92` - `ListLiteral.Type()` method
- `checker/checker.go:1877` - Expression checking for list literals  
- `vm/decode_module.go:260-297` - `one_of` implementation (currently limited)
- `vm/decode_test.go:674-688` - Test cases (some commented out due to this issue)

## Success Criteria

When fixed, these should work without panics:

```ard
use ard/decode

// Multiple built-in decoders of same return type
let string_decoders = [decode::string, decode::string]

// Mixed built-in and custom decoders
fn custom_string(data: decode::Dynamic) Str![decode::Error] { ... }
let mixed_decoders = [decode::string, custom_string]

// Complex compositional usage
let decoder = decode::one_of([
    decode::string,
    decode::field("name", decode::string),
    decode::nullable(decode::string)
])
```
