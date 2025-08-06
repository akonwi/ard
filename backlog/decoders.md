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

## 🚧 Phase 2: Compositional Decoders - IN PROGRESS

### ✅ Implemented: Nullable Decoder

```ard
// IMPLEMENTED - First compositional decoder
fn nullable<$T>(as: Decoder<$T>) MaybeDecoder<$T?>

// Usage:
let name_decoder = nullable(string())  // Handles null -> none(), "Alice" -> some("Alice")
```

### 🔄 Design Note: True Composition Like Gleam

Compositional decoders accept other decoders as parameters, enabling true composition:

```ard
// ✅ IMPLEMENTED
fn nullable<$T>(as: Decoder<$T>) MaybeDecoder<$T?>

// 🔄 PLANNED for Phase 2
fn field<$T>(key: Str, as: Decoder<$T>) Decoder<$T>
fn list<$T>(element_decoder: Decoder<$T>) Decoder<[$T]>
fn optional<$T>(as: Decoder<$T>, default: $T) Decoder<$T>
```

**Key Point**: Compositional decoders accept decoders for their elements, just like Gleam:
- ✅ `nullable(string())` - nullable string (null -> none(), "Alice" -> some("Alice"))
- 🔄 `list(string())` - list of strings
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

## 🔄 Current Status

- ✅ **Phase 1 Complete**: All primitive decoders working with proper error handling
- ✅ **First Compositional Decoder**: `nullable()` implemented and functional
- 🔄 **Type System**: Generic type resolution needs enhancement for complex compositional tests
- 🔄 **Next Phase**: Additional compositional decoders (field, list, optional)

## Migration Path

- ✅ **Backward Compatibility**: Existing `ard/json` remains unchanged
- ✅ **Gradual Adoption**: Users can use decode library for flexible JSON parsing
- ✅ **Foundation Ready**: Architecture supports external data (JSON, database rows, HTTP params)
- 🔄 **Future Enhancement**: Additional compositional capabilities (field, list, optional)

## Summary

The Ard decode library is now functional with:
- ✅ Complete primitive decoding (string, int, float, bool)
- ✅ Proper error handling with Ard type names
- ✅ Flexible external data parsing via `any()`
- ✅ First compositional decoder: `nullable()` 
- ✅ Comprehensive test coverage
- 🔄 Type system enhancements needed for full compositional testing

This validates the core concept and provides immediate value over the current `ard/json` approach.