# Ard Decode Library: Implementation Plan

## Overview

This document outlines the implementation of Gleam-style decoding for Ard, providing a flexible alternative to the current `ard/json` module that requires defining structs upfront. The decode library will enable type-safe transformation of external data (JSON, database rows, etc.) into Ard values using composable decoder functions.

## Phase 1: Core Primitives Only

### Core API Design

```ard
// ard/decode module

// Core decoder type - just a function
type Decoder<$T> = fn(Dynamic) $T![DecodeError]

// Use existing Dynamic type (alias for convenience)
type Dynamic = Any // Will use existing VM object system

// Error information with automatic path tracking
struct DecodeError {
  expected: Str,  // "Int", "String", etc.
  found: Str,     // "null", "Boolean", etc.
  path: [Str]     // ["user", "profile", "age"]
}
```

### Dynamic Data Access (Primitives Only)

```ard
// Core conversion functions (what primitive decoders use internally)
fn as_string(d: Dynamic) Str![DecodeError]
fn as_int(d: Dynamic) Int![DecodeError] 
fn as_float(d: Dynamic) Float![DecodeError]
fn as_bool(d: Dynamic) Bool![DecodeError]

// Primitive decoders - return single-error decoders
fn string() Decoder<Str> { as_string }
fn int() Decoder<Int> { as_int }
fn float() Decoder<Float> { as_float }
fn bool() Decoder<Bool> { as_bool }

// Entry point - accumulates errors from multiple decoders
fn decode<$T>(decoder: Decoder<$T>, data: Dynamic) $T![[DecodeError]] {
  // Converts single error to list for compositional error handling
  decoder(data)
}
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

## Future Phase 2: Compositional Decoders

### Design Note: True Composition Like Gleam

Compositional decoders should accept other decoders as parameters, enabling true composition:

```ard
// These will be implemented in Phase 2
fn field<$T>(key: Str, decoder: Decoder<$T>) Decoder<$T>
fn list<$T>(element_decoder: Decoder<$T>) Decoder<[$T]>
fn optional<$T>(decoder: Decoder<$T>, default: $T) Decoder<$T>
```

**Key Point**: Compositional decoders accept decoders for their elements, just like Gleam:
- `list(string())` - list of strings
- `field("name", string())` - string field named "name"  
- `optional(int(), 0)` - optional integer with default 0

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

### 4. File Structure

```
std_lib/
├── decode/
│   ├── decode.ard      # Core decoder functions (primitives only)
│   └── decode.go       # VM implementation of conversion functions
├── json/
│   ├── json.ard        # Add parse_to_dynamic function
│   └── json.go         # Implementation
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

## Implementation Steps

1. **Step 1**: Implement `DecodeError` struct in checker/VM
2. **Step 2**: Add `as_string`, `as_int`, `as_bool`, `as_float` builtin functions  
3. **Step 3**: Implement primitive decoder functions in `std_lib/decode/decode.ard`
4. **Step 4**: Add `parse_to_dynamic` to JSON module
5. **Step 5**: Add `decode` entry point function
6. **Step 6**: Write tests demonstrating primitive decoding
7. **Future**: Add compositional decoders (field, list, optional) in Phase 2

## Migration Path

- **Backward Compatibility**: Existing `ard/json` remains unchanged
- **Gradual Adoption**: Users can try decode library for specific use cases
- **Future Enhancement**: Phase 2 will add full compositional capabilities
- **External Data**: Architecture ready for database rows, HTTP params, etc.

This phased approach allows us to validate the core concept with primitives before building the full compositional system.