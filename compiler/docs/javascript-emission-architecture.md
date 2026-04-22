# JavaScript Emission Architecture

This document defines the intended architecture for Ard's JavaScript backend.

It exists as the reference for how JS output should be produced, formatted, and evolved over time.

## Goals

- Separate semantic lowering from output formatting.
- Keep the JS backend readable and maintainable as language support grows.
- Produce stable, deterministic JavaScript output.
- Make generated JS feel idiomatic to JavaScript users.
- Follow Prettier's formatting rules and layout philosophy where practical for the emitted JS subset.

## Pipeline

The JavaScript backend should follow this pipeline:

```text
parse AST
→ checker checked tree
→ JavaScript IR
→ JavaScript doc tree
→ printer
→ .mjs output
```

### Stage responsibilities

1. **Parse AST**
   - Ard source is parsed into the language AST.

2. **Checker checked tree**
   - The checker resolves names, types, modules, traits, matches, extern bindings, and target-specific rules.
   - This is the semantic source of truth for backend lowering.

3. **JavaScript IR**
   - The JS backend lowers the checked tree into a JS-specific intermediate representation.
   - This stage decides semantics and control flow.
   - Examples:
     - `try` lowering into explicit statement flow
     - enum lowering
     - match lowering
     - module import/export lowering
     - temp creation and sequencing
     - expression normalization when needed

4. **JavaScript doc tree**
   - JS IR is rendered into a document tree used for pretty-printing.
   - This stage decides presentation only.
   - It should not contain semantic lowering logic.

5. **Printer**
   - The doc tree is printed using width-aware pretty-printing rules.
   - Printing decisions should follow Prettier's JavaScript style where applicable.

## Architecture Principles

### 1. Lower first, format second

Semantic decisions belong in the JS IR lowering stage.
Formatting decisions belong in the doc/printer stage.

The doc tree should never be responsible for semantic transformations like:

- introducing temporaries
- normalizing `try`
- deciding evaluation order
- rewriting matches
- synthesizing control-flow structure

Those belong in JS IR lowering.

### 2. The checked tree is not the printer input

The backend should not render the checked tree directly into final JS text.

Instead:

```text
checked tree → JS IR → doc tree → text
```

This keeps the backend understandable and makes formatting improvements much easier.

### 3. The doc tree is presentation-only

The doc tree should exist to answer questions like:

- should this stay on one line?
- where should this wrap?
- how should this indent?
- when should trailing commas appear?
- how should imports, arrays, objects, args, and blocks be laid out?

It should not answer semantic questions.

### 4. Use a small JS IR for the emitted subset

The backend does not need a full general-purpose JavaScript compiler IR.

It only needs enough structure to represent the JavaScript Ard actually emits, such as:

- modules
- imports / exports
- functions
- classes / methods
- blocks
- `const` / `let`
- assignments
- `if` / `else`
- loops
- `return` / `throw`
- calls
- member access
- array literals
- object-like literals where needed
- `new` expressions
- binary / unary expressions

The JS IR should be minimal, typed enough for lowering, and easy to render into docs.

## Formatting Standard

### Source of truth

For generated JavaScript formatting, the backend should follow **Prettier** as the style reference wherever practical for the emitted subset.

This means:

- Prettier-like line breaking
- Prettier-like indentation
- Prettier-like block and brace layout
- Prettier-like wrapping for arguments, arrays, objects, imports, and exports
- Prettier-like preference for stable, low-configuration output

### Important nuance

The compiler should **not** shell out to Prettier or depend on a full external JS formatter.

Instead, Ard should:

- own its JS doc tree and printer
- implement formatting rules that intentionally match Prettier for the generated subset

So Prettier is the formatting model, not a runtime dependency.

## Shared Pretty-Printing Core

Ard already includes a doc-based formatter for Ard source code.

That pretty-printing core should be reused or extracted into a shared internal package so that both:

- Ard source formatting
- JavaScript output formatting

can use the same document primitives and printer algorithm.

Examples of reusable concepts:

- text
- concat
- group
- indent
- line
- soft line
- hard line
- conditional break rendering
- width-aware `fits(...)` decisions

The JS backend should build on this shared mechanism rather than inventing a second unrelated printer.

## Relationship to Current JS Work

This architecture fits the current backend direction:

- JS modules now emit as real ESM files.
- Ard `try` now lowers into ordinary statement flow instead of sentinel throw/catch plumbing.

Those changes make a JS IR + doc tree architecture especially valuable, because the backend now has a clearer separation between:

- semantic lowering
- printed output shape

## Initial Scope for JS IR → Doc Rendering

The first doc-based JS rendering pass should prioritize the highest-value emitted constructs:

1. imports / exports
2. function declarations
3. class declarations and methods
4. blocks
5. `if` / `else`
6. return / throw
7. variable declarations and assignment
8. function calls and argument lists
9. arrays and maps
10. enum object literals
11. common lowered `try` guard flow

After that, the backend can expand to improve formatting for:

- match lowering output
- nested callbacks
- chained calls
- helper IIFEs that still remain in some emitted paths

## Non-Goals

This architecture does **not** require:

- full JavaScript parsing after emission
- a general-purpose JS formatter for arbitrary handwritten JS
- exact bit-for-bit parity with Prettier for every possible JavaScript edge case

The goal is:

- a clean lowering pipeline
- a readable JS IR
- a Prettier-like formatting result for the JS Ard actually generates

## Suggested File/Layer Split

A reasonable structure for the backend is:

```text
compiler/javascript/
  javascript.go   // build/run orchestration
  lower.go        // checked tree -> JS IR
  ir.go           // JS IR node definitions
  docs.go         // JS IR -> doc tree
  printer.go      // JS-specific printing glue
```

And the generic document model / pretty-printer should live in a reusable internal location shared with the Ard formatter.

## Rollout Recommendation

1. Extract or share the existing doc printer core used by the Ard formatter.
2. Introduce a minimal JS IR for the generated subset.
3. Move JS emission from direct string-building toward:
   - checked tree → JS IR
   - JS IR → doc tree
4. Make JS formatting decisions intentionally follow Prettier for the emitted subset.
5. Add snapshot-style tests for generated JS layout.
6. Migrate the highest-value constructs first, then expand incrementally.

## Summary

The JavaScript backend should be structured as:

```text
checked Ard tree
→ JS IR
→ doc tree
→ Prettier-like printer
→ output JS
```

That is the intended long-term architecture and should be treated as the reference model for future JS backend work.
