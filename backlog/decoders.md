# Ard Decode Library: Implementation Status

## Overview

This document tracks the implementation of Gleam-style decoding for Ard, providing a flexible alternative to the current `ard/json` module that requires defining structs upfront. The decode library enables type-safe transformation of external data (JSON, database rows, etc.) into Ard values using composable decoder functions.

## ✅ Phase 1: Core Primitives - COMPLETED

### Core API Design

```ard
// ard/decode module - IMPLEMENTED

// Core decoder type - just a function
type Decoder<$T> = fn(Dynamic) $T![Error]

// Dynamic type for external/untyped data
type Dynamic = Dynamic // Opaque type implemented in VM

// Error information with path tracking
struct Error {
  expected: Str,  // "Str", "Int", "Bool", etc. (uses Ard type names)
  found: Str,     // "Void", "Dynamic", etc.
  path: [Str]     // ["user", "profile", "age"] - empty for primitives
}
```

### ✅ Implemented Primitive Decoders

```ard
// Core conversion functions (what primitive decoders use internally) - IMPLEMENTED
fn as_string(d: Dynamic) Str![Error]
fn as_int(d: Dynamic) Int![Error] 
fn as_float(d: Dynamic) Float![Error]
fn as_bool(d: Dynamic) Bool![Error]

// Primitive decoders - IMPLEMENTED
fn string() Decoder<Str>
fn int() Decoder<Int> 
fn float() Decoder<Float>
fn bool() Decoder<Bool>

// Entry point function - IMPLEMENTED
fn decode<$T>(decoder: Decoder<$T>, data: Dynamic) $T![Error]

// External data parser - IMPLEMENTED
fn any(external_data: Str) Dynamic  // Parses JSON, invalid becomes nil
```

### Basic Usage Example (Primitives Only)

```ard
use ard/decode

// Decode primitive values from external data (JSON strings, database values, etc.)
let name_data = decode::any("\"Alice\"")
let name = decode::decode(decode::string(), name_data).expect("")
// name is "Alice"

let age_data = decode::any("30")  
let age = decode::decode(decode::int(), age_data).expect("")
// age is 30

let active_data = decode::any("true")
let active = decode::decode(decode::bool(), active_data).expect("")
// active is true

// Invalid data fails at decode time, not parse time
let invalid_data = decode::any("invalid json")  // Always succeeds
let result = decode::decode(decode::string(), invalid_data)  // Fails here
if result.is_err() {
    // Handle invalid data
}
```

## ✅ Phase 2: Compositional Decoders - COMPLETE

### ✅ Implemented: Nullable Decoder

```ard
// FULLY IMPLEMENTED - First compositional decoder
fn nullable<$T>(as: Decoder<$T>) NullableDecoder<$T?>

// Complete Implementation: ✅ WORKING
// - Type system correctly resolves generic parameters
// - VM correctly handles null -> none(), values -> some()  
// - Proper error propagation from inner decoders
// - Full test coverage with complex compositions

// Usage Example (fully functional):
let data = decode::any("\"hello\"")
let nullable_decoder = decode::nullable(decode::string())
let result = decode::decode(nullable_decoder, data)
result.expect("").or("default")  // Returns "hello"
```

### ✅ Implemented: List Decoder

```ard
// FULLY IMPLEMENTED - Second compositional decoder  
fn list<$T>(element_decoder: Decoder<$T>) ListDecoder<[$T]>

// Complete Implementation: ✅ WORKING
// - Type system correctly resolves generic parameters
// - VM decodes arrays by applying element decoder to each item
// - Accumulates errors with path information ([0], [1], etc.)
// - Handles empty arrays and mixed valid/invalid elements
// - Full test coverage including error cases

// Usage Example (fully functional):
let data = decode::any("[1, 2, 3, 4, 5]")
let list_decoder = decode::list(decode::int())
let result = decode::decode(list_decoder, data)
let list = result.expect("")
list.size()  // Returns 5
```

### ✅ Implemented: Map Decoder

```ard
// FULLY IMPLEMENTED - Third compositional decoder with key/value support
fn map<$K, $V>(key: Decoder<$K>, val: Decoder<$V>) MapDecoder<[$K:$V]>

// Complete Implementation: ✅ WORKING
// - Type system correctly resolves dual generic parameters K and V
// - VM decodes JSON objects by applying key decoder to keys, value decoder to values
// - Supports flexible key types (Str, Int, Float, Bool) with automatic string conversion
// - Enhanced error reporting distinguishes between key and value decoding errors
// - Full test coverage including error cases and compositional patterns

// Usage Examples (fully functional):
let data = decode::any("{\"name\": \"Alice\", \"age\": \"30\"}")

// String keys to string values
let string_map_decoder = decode::map(decode::string(), decode::string())
let result1 = decode::decode(string_map_decoder, data)
let map1 = result1.expect("")
map1.get("name").or("default")  // Returns "Alice"

// String keys to integer values  
let mixed_map_decoder = decode::map(decode::string(), decode::int())
let result2 = decode::decode(mixed_map_decoder, data)
// Would fail because "Alice" can't be decoded as Int, proper error with path info

// Compositional with nullable values
let nullable_map_decoder = decode::map(decode::string(), decode::nullable(decode::string()))
// Handles: {"name": "Alice", "nickname": null, "city": "Boston"}
```

### ✅ Generic Type System: Fixed!

**Resolution**: The generic type limitation has been **successfully resolved** through a restructuring of the function type definitions.

**The Fix**: Instead of using unresolvable `Any` types, the checker now uses concrete `FunctionDef` structures with shared generic parameters:

```go
// Before (broken): &Any{name: "MaybeDecoder"} 
// After (working):
innerT := &Any{name: "T"}  // Shared generic parameter
innerDecoder := &FunctionDef{
    ReturnType: MakeResult(innerT, MakeList(DecodeErrorDef)),  // Uses T
}
nullableDecoder := &FunctionDef{
    ReturnType: MakeResult(MakeMaybe(innerT), ...), // Uses Maybe<T> - same T!
}
```

**Result**: The type system can now properly resolve generic types through function composition chains:
1. `decode::string()` → `Decoder<Str>`  
2. `decode::nullable(string_decoder)` → `NullableDecoder<Str?>`
3. `result.expect("")` → works because result type is `Result<Str?, [Error]>`

**Status**: 
- ✅ **Type checking**: **COMPLETE** - all compositional patterns work
- ✅ **VM implementation**: **COMPLETE** - runtime behavior is correct
- ✅ **Testing**: **COMPLETE** - comprehensive test coverage achieved

### 🔄 Design Note: True Composition Like Gleam

Compositional decoders accept other decoders as parameters, enabling true composition:

```ard
// ✅ IMPLEMENTED
fn nullable<$T>(as: Decoder<$T>) MaybeDecoder<$T?>

// ✅ IMPLEMENTED 
fn list<$T>(element_decoder: Decoder<$T>) Decoder<[$T]>

// ✅ IMPLEMENTED
fn map<$K, $V>(key: Decoder<$K>, val: Decoder<$V>) Decoder<[$K:$V]>

// 🔄 PLANNED for Phase 3
fn field<$T>(key: Str, as: Decoder<$T>) Decoder<$T>
fn optional<$T>(as: Decoder<$T>, default: $T) Decoder<$T>
```

**Key Point**: Compositional decoders accept decoders for their elements, just like Gleam:
- ✅ `nullable(string())` - nullable string (null -> none(), "Alice" -> some("Alice"))
- ✅ `list(string())` - list of strings  
- ✅ `map(string(), int())` - map with string keys and integer values
- 🔄 `field("name", string())` - string field named "name"  
- 🔄 `optional(int(), 0)` - optional integer with default 0

This enables building complex decoders from simple ones:

```ard
// Future example - person decoder built from primitives
fn person_decoder() Decoder<Person> {
  fn(data: Dynamic) {
    let name = try field("name", string())(data)
    let age = try field("age", int())(data)
    let hobbies = try field("hobbies", list(string()))(data)
    
    Person { name: name, age: age, hobbies: hobbies }
  }
}
```

## Implementation Strategy

### 1. Use Existing VM Infrastructure

**No New Object Types Needed:**
- Use existing `object` struct in VM instead of introducing `DynamicObject`
- Dynamic type is just an alias for convenience
- Leverage existing type inspection capabilities

**VM Implementation:**
```go
// vm/builtins.go - implement as builtin functions
func asDynamicString(args ...*object) *object {
    obj := args[0]
    // Type checking and conversion logic using existing object types
    // Returns Result<String, DecodeError>
}

func asDynamicInt(args ...*object) *object {
    obj := args[0] 
    // Convert to integer, return Result
}

// etc. for other primitives
```

### 2. JSON Integration

**Add Dynamic Support to Decode Module:**
```ard
// ard/decode module functions
fn any(external_data: Str) Dynamic {
  // Implementation parses external data (JSON, CSV, XML, etc.) into Dynamic objects
  // Currently supports JSON parsing, but extensible to other formats
  // Invalid data becomes nil Dynamic, errors surface at decode time
}

// JSON module keeps existing decode for backward compatibility  
fn decode<$T>(json_str: Str) $T![ParseError] {
  // Current implementation unchanged in json module
}
```

### 3. Error Handling

**DecodeError Structure:**
```ard
struct DecodeError {
  expected: Str,  // "Int", "String", etc.
  found: Str,     // What we actually found
  path: [Str]     // Path to error (for Phase 2)
}
```

**Phase 1 Error Messages:**
```ard
// Simple errors for primitives
DecodeError {
  expected: "Int",
  found: "String",
  path: []  // Empty for now, will be used in Phase 2
}
```

### 4. ✅ Implemented File Structure

```
checker/
├── decode.go           # ✅ DecodePkg with Error struct, Dynamic type, function signatures
vm/
├── decode_module.go    # ✅ DecodeModule with primitive + nullable decoder implementations
├── decode_test.go      # ✅ Comprehensive tests for primitives and nullable decoder
```

## Benefits Over Current ard/json (Even With Just Primitives)

1. **Flexible Data Access**: Work with arbitrary JSON structure without predefined types
2. **Uniform Interface**: Same pattern will extend to complex types
3. **Explicit Error Handling**: Clear about what can fail and how
4. **Foundation for Composition**: Sets up the architecture for complex decoders
5. **Multiple Data Sources**: Same interface will work with JSON, database rows, etc.

## Usage Comparison

### Current ard/json (Requires Known Structure)
```ard
use ard/json

// Must know the exact structure and define types upfront
struct Response { success: Bool, count: Int, message: Str }
let data = json::decode<Response>(json_string).expect("")
```

### New ard/decode (Phase 1 - Flexible Access)  
```ard
use ard/decode

// Can extract individual values from external data without knowing structure
let json_obj = decode::any(json_string)

// Extract just what you need
let success = decode::decode(decode::bool(), json_obj).expect("")
let count = decode::decode(decode::int(), json_obj).expect("")

// Or handle errors gracefully
let message = decode::decode(decode::string(), json_obj).or("No message")

// Works with any external data format - JSON, CSV, XML, database values, etc.
let database_value = decode::any(row_data)
let parsed_value = decode::decode(decode::string(), database_value).expect("")
```

## ✅ Completed Implementation Steps

1. ✅ **Step 1**: Implement `Error` struct in checker/VM
2. ✅ **Step 2**: Add `as_string`, `as_int`, `as_bool`, `as_float` builtin functions  
3. ✅ **Step 3**: Implement primitive decoder functions in VM DecodeModule
4. ✅ **Step 4**: Add `any()` function to decode module (parses JSON to Dynamic)
5. ✅ **Step 5**: Add `decode()` entry point function
6. ✅ **Step 6**: Write comprehensive tests demonstrating primitive decoding
7. ✅ **Step 7**: Add first compositional decoder: `nullable()`
8. ✅ **Step 8**: Add second compositional decoder: `list()`
9. ✅ **Step 9**: Add third compositional decoder: `map()` with key/value support

## ✅ Current Status

- ✅ **Phase 1 Complete**: All primitive decoders working with proper error handling
- ✅ **Phase 2 Complete**: Three compositional decoders fully implemented: `nullable()`, `list()`, and `map()`
- ✅ **Type System Fixed**: Generic type resolution now works correctly for all compositional patterns
- 🔄 **Next Phase**: Additional compositional decoders (field, optional) for object decoding

## Migration Path

- ✅ **Backward Compatibility**: Existing `ard/json` remains unchanged
- ✅ **Gradual Adoption**: Users can use decode library for flexible JSON parsing
- ✅ **Foundation Ready**: Architecture supports external data (JSON, database rows, HTTP params)
- 🔄 **Future Enhancement**: Additional compositional capabilities (field, list, optional)

## Summary

The Ard decode library is now fully functional with:
- ✅ **Complete primitive decoding** (string, int, float, bool)
- ✅ **Proper error handling** with Ard type names and path tracking
- ✅ **Flexible external data parsing** via `any()`
- ✅ **Three compositional decoders**: `nullable()`, `list()`, and `map()` with full type safety
- ✅ **Comprehensive test coverage** for all implemented functionality
- ✅ **Generic type system** working correctly for all compositional patterns

**Key Achievement**: Full compositional decoder functionality now works end-to-end, from type checking through VM execution. The architecture demonstrates sophisticated generic type resolution that enables true Gleam-style decoder composition.

**Immediate Value**: The library provides significant advantages over current `ard/json`:
- **Flexible data access** without requiring predefined struct definitions
- **Composable decoders** that can be combined for complex data patterns
- **Type-safe error handling** with detailed path information
- **Extensible architecture** ready for additional decoder types