---
title: Decoding external data
description: Parse data into Ard values
---

# Data Decoding with ard/decode

The `ard/decode` module provides a flexible, type-safe way to decode external data from multiple sources (JSON, SQLite query results, etc.) without requiring predefined struct definitions. It uses composable decoder functions inspired by Gleam's approach.

## Overview

Unlike `ard/json` which requires defining structs upfront, the decode library lets you work with arbitrary JSON structures and extract only the data you need.

```ard
use ard/decode

// Parse any JSON string into Dynamic data
let data = decode::any("{\"name\": \"Alice\", \"age\": 30}")

// Extract specific values with type safety
let name = decode::run(data, decode::field("name", decode::string)).expect("")
let age = decode::run(data, decode::field("age", decode::int)).expect("")
```

## Core Concepts

### Dynamic Type

The `Dynamic` type represents external, untyped data (JSON, database rows, etc.):

```ard
let data = decode::any("{\"users\": [\"Alice\", \"Bob\"]}")  // Dynamic
```

### Decoders

Decoders are functions that transform `Dynamic` data into typed Ard values:

```ard
type Decoder($T) = fn(Dynamic) $T![Error]
```

## Basic Decoders

### Primitive Decoders

```ard
use ard/decode

let data = decode::any("\"hello\"")
let text = decode::run(data, decode::string).expect("")  // "hello"

let data = decode::any("42")
let number = decode::run(data, decode::int).expect("")   // 42

let data = decode::any("3.14")
let pi = decode::run(data, decode::float).expect("")     // 3.14

let data = decode::any("true")
let active = decode::run(data, decode::bool).expect("")  // true
```

### Entry Point Function

The `decode::run()` function applies a decoder to dynamic data:

```ard
fn run(data: Dynamic, decoder: Decoder($T)) $T![Error]
```

## Compositional Decoders

The power of the decode library comes from composing simple decoders into complex ones.

### Nullable Values

Handle optional or null values using `nullable()`:

```ard
let data = decode::any("null")
let maybe_text = decode::run(data, decode::nullable(decode::string)).expect("")
let text = maybe_text.or("default")  // "default"

let data = decode::any("\"hello\"")
let maybe_text = decode::run(data, decode::nullable(decode::string)).expect("")
let text = maybe_text.or("default")  // "hello"
```

### Lists

Decode arrays using `list()`:

```ard
let data = decode::any("[1, 2, 3, 4, 5]")
let numbers = decode::run(data, decode::list(decode::int)).expect("")
numbers.size()  // 5
```

### Maps

Decode objects with flexible key and value types using `map()`:

```ard
let data = decode::any("\{\"name\": \"Alice\", \"city\": \"Boston\"\}")

// String keys to string values
let info = decode::run(data, decode::map(decode::string, decode::string)).expect("")
info.get("name").or("unknown")  // "Alice"
```

### Field Extraction

Extract specific fields from objects using `field()`:

```ard
let json = "\{\"user\": \{\"name\": \"Alice\", \"age\": 30\}\}"
let data = decode::any(json)

// Extract nested field
let name = decode::run(data,
  decode::field("user",
    decode::field("name", decode::string)
  )
).expect("")  // "Alice"
```

## Advanced Patterns

### Composition

Combine decoders for complex data structures:

```ard
use ard/decode

let json = "\{\"users\": [\{\"name\": \"Alice\", \"active\": true\}, \{\"name\": \"Bob\", \"active\": null\}]\}"
let data = decode::any(json)

// Decode array of user objects
let user_decoder = decode::map(decode::string, decode::nullable(decode::bool))
let users = decode::run(data, decode::field("users", decode::list(user_decoder))).expect("")

// Access user data
let first_user = users.at(0)
let is_active = first_user.get("active").or(maybe::some(false))
```

### Error Handling

The decode library provides detailed error information:

```ard
let data = decode::any("\{\"age\": \"not_a_number\"\}")
let result = decode::run(data, decode::field("age", decode::int))

match result {
  ok(age) => io::print("Age: {age}"),
  err(errors) => {
    let first_error = errors.at(0)
    io::print("Expected: {first_error.expected}")  // "Int"
    io::print("Found: {first_error.found}")        // "Dynamic"
    io::print("Path: {first_error.path}")          // ["age"]
  }
}
```

## Real-World Example

Here's how to process API responses without predefined structs:

```ard
use ard/http
use ard/decode
use ard/io

fn fetch_pokemon() {
  let response = http::get("https://pokeapi.co/api/v2/pokemon").expect("Request failed")

  if response.is_ok() {
    let data = decode::any(response.body)

    // Extract count
    let count = decode::run(data, decode::field("count", decode::int))
    match count {
      ok(n) => io::print("Total Pokemon: {n}"),
      err(_) => io::print("Could not extract count")
    }

    // Extract results array
    let results_decoder = decode::list(decode::map(decode::string, decode::string))
    let results = decode::run(data, decode::field("results", results_decoder))
    match results {
      ok(pokemon_list) => io::print("Found {pokemon_list.size()} Pokemon"),
      err(_) => io::print("Could not extract results")
    }
  }
}
```

## Benefits

- **No struct definitions required** - work with arbitrary JSON
- **Type safety** - decoders ensure correct types
- **Composable** - build complex decoders from simple ones
- **Flexible** - extract only what you need
- **Error-friendly** - detailed error messages with path information
- **Extensible** - works with JSON, database rows, and other external data

The decode library is perfect for working with APIs, configuration files, and any situation where you need flexible JSON processing without rigid type definitions.
