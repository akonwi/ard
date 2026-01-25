# AGENTS.md

This file provides guidance to a coding agent when working with code in this repository.

## Project Overview

This repository contains a Tree-sitter grammar for the Ard programming language. Tree-sitter is an incremental parsing system that generates language parsers. This grammar defines the syntax for the Ard language, enabling features like syntax highlighting, code folding, and more in text editors that support Tree-sitter.

## Commands

### Building and Generating the Parser

```bash
# Generate the parser from grammar.js into C code and other parser files
npm run generate

# Build and launch the Tree-sitter playground to test the grammar interactively
npm run start

# Build for WebAssembly
npm run prestart
```

### Testing

```bash
# Run all tests
npm test

# Update test expectations based on current implementation
npm run test:update

# Test the Go bindings specifically
npm run test:binding
```

### Installation

```bash
# Install dependencies
npm install

# Install the generated parser
npm run install
```

## Project Structure

- `grammar.js`: The main grammar definition file written in JavaScript. This defines the syntax of the Ard language using Tree-sitter's DSL.
- `src/`: Contains the generated parser code and related files.
- `test/corpus/`: Contains test files that define examples of Ard syntax and their expected parse trees.
- `bindings/`: Contains language bindings for various platforms:
  - `go/`: Go language bindings
  - `node/`: Node.js bindings
  - `c/`: C language bindings
  - `rust/`: Rust language bindings

## Language Features

The Ard language grammar includes support for:

- Variables with `let` and `mut` bindings
- Functions with parameters and return types
- Structs and methods via `impl` blocks
- Enums and type unions
- Control flow (if/else, match expressions)
- Loops (for, while, for-in)
- Lists and maps
- String interpolation
- Comments (line and block)
- Modules and imports

## Working with the Grammar

When modifying the grammar:

1. Edit the `grammar.js` file to add or modify language syntax rules
2. Run `npm run generate` to update the parser
3. Add or update tests in the `test/corpus/` directory
4. Run `npm test` to ensure the changes parse correctly
5. Use `npm run start` to test interactively in the playground

The grammar uses Tree-sitter's conflict resolution system with `precedences` and `conflicts` to handle ambiguous grammar constructs.

## Additional Notes

- Also consider the AGENTS.md in the parent compiler directory
