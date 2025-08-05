---
title: Introduction
description: Learn about Ard's philosophy, design goals, and key features.
---

## What's Ard?

- **A** **R**-eally **D**-ope Language
- [ard is slang for "alright."](https://www.dictionary.com/e/slang/ard/)
- [Irish and Scottish Gaelic word meaning 'high, lofty', 'above the ground, elevated'](https://www.oxfordreference.com/display/10.1093/oi/authority.20110803095422661)
- Ardbeg is my favorite scotch and I was drinking it when I came up with this name

## Language Description

Ard is a general-purpose programming language designed for legibility, simplicity, and type-safety.
It combines elements from JavaScript, Swift, Go, and Rust.

## Design Goals

- **Readability**: Ard code should be easy to read and understand.
- **Simple**: There should be one obvious way to do things.
- **Safety**: Type errors are caught at compile time and runtime errors must be handled.
- **Reliable**: Built on Go's runtime, so programs can be fast and efficient.

## Philosophy

Ard follows Go's philosophy for readability from left to right, rather than the usual spiraling syntax found in C-based languages ([read more](https://c-faq.com/decl/spiral.anderson.html)). This design choice makes code more intuitive to read and understand.

## Key Characteristics

### No Exceptions
Ard does not have exceptions. Instead, errors are represented as values using the built-in `Result<$Val, $Err>` type.
This approach makes error handling explicit and facilitates fault-tolerant programming.

### Static Typing with Inference
Variables and functions are statically typed. The compiler can also infer types in most cases, reducing verbosity while maintaining safety.

### Explicit Mutability
Ard's memory management is focused on data access controls. Variables and data structures are immutable by default and must be marked as mutable to prevent unwanted data mutations.

### No Return Keyword
Without return keywords, the returned value of a function is always the last expression when a function signature states the return type.
