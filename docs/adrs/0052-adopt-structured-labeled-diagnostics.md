# 0052: Adopt Structured Labeled Diagnostics

## Status

Accepted

## Context

Ard checker diagnostics currently contain a severity, one message, one file path, and one parser location:

```go
type Diagnostic struct {
    Kind     DiagnosticKind
    Message  string
    filePath string
    location parse.Location
}
```

The command-line compiler renders this as a single line containing the file, start position, and message. The LSP converts the same fields to one editor range and one message. This is enough to report that checking failed, but it cannot explain relationships such as:

- an expression has one type while an annotation requires another;
- a mutation occurs here while the immutable binding was declared elsewhere;
- an argument is invalid because of a parameter declared in another module;
- a declaration duplicates an earlier declaration.

These errors are more useful when the compiler can label both the immediate failure and the source of the constraint.

Rust and Gleam both produce source-aware diagnostics with a concise title and labeled spans. Rust has a broad system that also includes stable error codes, child notes, help, structured suggestions, applicability levels, and machine-readable output. Gleam uses a smaller core model: title, explanatory text, severity, a primary labeled source span, optional secondary labeled spans, and an optional hint. Gleam's renderer is separate from diagnostic construction.

Ard does not need to adopt either compiler's entire feature set. The immediate goal is to make checker feedback precise and understandable by representing the relationships the checker already knows and rendering them against source text.

Two correctness constraints must be addressed as part of that foundation:

1. `parse.Location` does not yet have a clearly documented contract for end positions, column encoding, empty spans, and synthetic spans. Rich underlining and correct LSP conversion require such a contract.
2. A checker result may include diagnostics originating in imported files. The LSP currently publishes all converted diagnostics under the document being analyzed rather than grouping or filtering them by the diagnostic's actual file path.

## Decision

Ard will represent checker feedback as structured, source-aware diagnostics with a concise title, explanatory text, one primary labeled span, and zero or more secondary labeled spans.

The target core model is conceptually:

```go
type Diagnostic struct {
    Severity Severity
    Title    string
    Text     string // optional explanatory text
    Primary  Label
    Related  []Label
}

type Label struct {
    Span    SourceSpan
    Message string
}

type SourceSpan struct {
    FilePath string
    Location parse.Location
}
```

The exact Go API may differ as implementation reveals naming or ownership concerns, but it must preserve these properties:

- file identity and location travel together as a source span;
- one label is identified as the primary failure location;
- secondary labels may refer to the same file or another file;
- title and optional explanatory text are distinct from labels;
- diagnostics contain structured data, not pre-rendered terminal output;
- terminal and LSP presentation are adapters over the same diagnostic data;
- all emitters adapt into one canonical representation rather than maintaining parallel legacy and structured meanings.

The source text itself will not be stored redundantly in every diagnostic. Consumers must be able to retrieve the matching source from disk, compiler-loaded modules, test fixtures, or LSP overlays. The ownership and API for that retrieval will be determined during implementation.

### Source location contract

Before rich rendering is relied upon, Ard will define and test a single source-location contract covering:

- whether an end position is inclusive or exclusive;
- whether columns count bytes, Unicode scalar values, or another unit;
- how empty and synthetic spans are represented;
- normalization of missing or invalid end positions;
- conversion to LSP's zero-based UTF-16 positions.

Parser, checker, terminal renderer, and LSP code will use shared conversion and normalization helpers rather than defining their own interpretations. Ard's internal column unit need not be UTF-16; conversion to UTF-16 is source-aware work performed at the LSP boundary.

### Multi-file diagnostics

Diagnostic consumers must respect each source span's file identity.

The LSP must not publish a diagnostic at a location in the wrong document. An initial implementation may filter results to the document being published; a project-authoritative implementation may instead group them by primary source URI. The exact ownership, versioning, stale-result, and clearing strategy for diagnostics produced by multiple analyses is deferred until implementation.

The primary span determines the LSP range. The adapter composes the LSP message from the title, optional text, and primary label rather than discarding any of them. Secondary cross-file labels may map to LSP related information when supported. The main diagnostic must remain understandable when a client does not support related information.

### Rendering

The command-line compiler will gain a source-aware renderer capable of showing:

- severity and title;
- file and source position;
- relevant source lines;
- an emphasized primary span and its label;
- secondary spans and their labels;
- explanatory text outside the source excerpt.

Color and terminal decoration are renderer concerns. A deterministic plain-text mode will support tests, redirected output, and terminals without color. The exact layout, rendering algorithm, implementation library, and resemblance to Rust or Gleam output are not decided by this ADR.

The LSP adapter will continue to use native LSP diagnostics rather than sending terminal-rendered text.

### Incremental migration

Existing checker emitters will not be rewritten all at once. A compatibility path may construct a structured diagnostic from the existing severity, message, file, and location fields while diagnostic families migrate individually.

The first reference implementation will be type mismatch diagnostics. A useful result should be able to distinguish the incompatible expression from the source of the expected type:

```text
error: Type mismatch
  ┌─ main.ard:4:7
  │
4 │   let name: Str = 42
  │             ---   ^^ this expression has type `Int`
  │             │
  │             this annotation requires `Str`
```

The first milestone is intentionally an end-to-end vertical slice rather than only a checker data-model change. It must exercise the complete path:

1. checker construction of structured labels;
2. source retrieval;
3. rich command-line rendering;
4. correct LSP publication;
5. focused structured and rendering tests.

After the reference diagnostic establishes the API, other high-value families can migrate independently, including unknown names, duplicate declarations, immutable mutation, incorrect function arguments, missing fields or methods, and import resolution errors.

### Migration checklist

This checklist tracks migration from compatibility diagnostics (`addError`, `addWarning`, and direct `NewDiagnostic` adaptation) to typed structured diagnostics. Check an item only when its relevant emitters use the canonical representation and focused tests cover their labels and source spans. A checked family may still gain new diagnostics later; it means the legacy emitters known at the time of migration have been addressed.

#### Completed foundation and families

- [x] Canonical diagnostic, label, and source-span representation
- [x] Compatibility path for unmigrated diagnostics
- [x] Source-aware plain-text CLI rendering
- [x] Unicode-, wide-character-, combining-character-, and tab-aware terminal label alignment
- [x] Source-aware UTF-16 LSP range conversion
- [x] Multi-file LSP filtering and related information
- [x] Type-mismatch reference vertical slice
- [x] Unknown lexical variables and functions
- [x] Unknown fields, methods, and accessor-chain members
- [x] Unknown types, traits, modules, namespaces, qualified members, Go entities, enum variants, static members, constructors, assignment targets, and struct types
- [x] Duplicate top-level type declarations
- [x] Duplicate struct-field declarations
- [x] Duplicate imports
- [x] Direct immutable-variable assignment
- [x] Incorrect function-argument types, including cross-file Ard parameter provenance and synthetic Go/builtin parameters

#### Remaining checker migrations

- [x] **Import and module resolution**
  - [x] Failed Ard and Go imports
  - [x] Circular dependencies
  - [x] Module loading failures

- [x] **Type declarations and recursive types**
  - [x] Redeclaration of built-in types
  - [x] Recursive aliases
  - [x] Infinite-size recursive fields

- [x] **Generic type resolution**
  - [x] Unresolved and unbound generics
  - [x] Specialization of non-generic types
  - [x] Incorrect and missing type arguments
  - [x] Unsupported recursive generic references
  - [x] Method-introduced generic parameters

- [x] **Type validity**
  - [x] Invalid map key types
  - [x] Malformed internal type nodes

- [x] **Remaining type mismatches**
  - [x] Branch and return mismatches
  - [x] Assignment and property mismatches
  - [x] List, fixed-array, and map mismatches
  - [x] Struct-literal field mismatches
  - [x] `Result`, `Maybe`, and `try` mismatches
  - [x] Interface and trait parameter mismatches
  - [x] String interpolation mismatches

- [x] **Mutable references and indirect mutation**
  - [x] Mutable references to immutable values
  - [x] Reference rebinding and unreachable referent assignment
  - [x] Immutable property assignment
  - [x] Mutating calls and pointer-receiver calls on immutable values
  - [x] Assignment to Go constants and static properties

- [x] **Function and method call shape**
  - [x] Calling non-functions
  - [x] Incorrect argument counts and missing parameters
  - [x] Unknown named arguments
  - [x] Named arguments on unsupported Go and foreign calls
  - [x] Invalid function type arguments

- [ ] **Function returns, closures, and tests**
  - [x] Function-body return mismatches
  - [x] Value-producing `if` without `else`
  - [ ] Invalid test placement, parameters, generics, and return types

- [ ] **Traits, interfaces, and implementations**
  - [ ] Invalid implementation targets
  - [ ] Methods absent from a trait or interface
  - [ ] Incorrect method parameter counts and mutability
  - [ ] Incorrect method return types
  - [ ] Missing required methods
  - [ ] Duplicate methods
  - [ ] Mutating enum methods

- [ ] **Enums**
  - [ ] Empty enums
  - [ ] Duplicate variants
  - [ ] Non-integer discriminants
  - [ ] Duplicate discriminant values

- [ ] **Collection and struct literals**
  - [ ] Empty untyped lists and maps
  - [x] Fixed-array length mismatches and mixed list element types
  - [x] Map key and value mismatches
  - [ ] Repeated fields in struct literals
  - [ ] Missing struct fields
  - [ ] Invalid struct type arguments and Go struct literal forms

- [ ] **Go FFI and generic Go entities**
  - [ ] Incorrect Go type-argument counts
  - [ ] Conflicting or uninferable type arguments
  - [ ] Unsatisfied Go constraints
  - [ ] Unsupported Go functions, methods, fields, constants, and variables
  - [ ] Invalid generic Go type instantiation
  - [ ] Generic Go functions referenced as values

- [ ] **Control flow**
  - [ ] Invalid `defer` placement and form
  - [ ] Invalid `break`
  - [ ] Non-boolean loop conditions
  - [ ] Invalid `for` initialization and update
  - [ ] Invalid ranges and unsupported iteration targets

- [ ] **Operators and comparisons**
  - [ ] Invalid unary negation
  - [ ] Arithmetic between incompatible types
  - [ ] Unsupported operand types
  - [ ] Invalid comparisons and boolean operations
  - [ ] Chained equality operators

- [ ] **Conditionals**
  - [ ] Non-boolean conditions
  - [x] Inconsistent branch result types
  - [x] Missing `else` when used as a value

- [ ] **Match and pattern diagnostics**
  - [ ] Invalid pattern forms
  - [ ] Duplicate cases and catch-all cases
  - [ ] Missing exhaustive cases
  - [ ] Invalid foreign-type patterns
  - [ ] Enum, union, boolean, string, rune, and integer match errors
  - [ ] Invalid and duplicate `select` arms
  - [ ] Ignored-pattern warnings

- [ ] **`try`, `Result`, and `Maybe` propagation**
  - [ ] Invalid `try` contexts, including deferred work
  - [x] Incompatible enclosing return types
  - [ ] Invalid catch and result error types
  - [ ] Invalid `Maybe::new` arguments

- [ ] **Literal and conversion validation**
  - [ ] Invalid rune, integer, and float literals
  - [ ] Numeric overflow
  - [ ] Invalid `Str::from` and numeric conversions
  - [x] Non-stringable interpolation values

- [ ] **Unsafe API usage**
  - [ ] Missing `ard/unsafe` imports
  - [ ] Invalid `unsafe::cast` arguments and type arguments
  - [ ] Invalid `unsafe::is_nil` arguments and type arguments

- [ ] **Opaque helper-generated errors**
  - [ ] Replace generic binding, argument resolution, and call-resolution `err.Error()` emissions with typed helper errors and diagnostics

#### Diagnostics outside the checker

- [ ] Adapt parser diagnostics into the canonical structured representation for LSP publication
- [ ] Represent relevant frontend and resolver failures as structured diagnostics where source spans are available

## Consequences

- Checker diagnostics can explain both where a failure occurs and why a constraint exists.
- Cross-file relationships can be represented without embedding file references in prose.
- CLI and LSP output derive from one semantic diagnostic model while retaining presentation appropriate to each medium.
- Diagnostic wording and rendering can evolve independently from checker control flow.
- Source-location semantics become an explicit compiler contract rather than an assumption duplicated by consumers.
- LSP publication must account for diagnostics from multiple files and track files whose diagnostics need clearing.
- Existing checker tests that compare complete message strings will migrate gradually toward assertions over structured fields.
- Diagnostic consumers need reliable, overlay-aware source retrieval, adding some architectural complexity.
- Source identity needs canonical semantics and may eventually require an opaque identity rather than a raw file path.
- Multi-file LSP publication introduces ownership, versioning, stale-result, and clearing concerns when multiple analyses report against the same URI.
- Diagnostic and label ordering must be deterministic for command-line output and tests.
- During migration, legacy emitters will be adapted into the canonical structured representation.

## Deferred Decisions

The following capabilities are not part of the core model or the first milestone:

- notes;
- help or hint messages;
- stable diagnostic codes;
- structured text edits;
- suggestion applicability levels;
- LSP code actions generated from checker diagnostics;
- machine-readable command-line diagnostic output.

They may be added independently when concrete user or tooling needs justify them. This ADR intentionally does not commit Ard to Rust's complete diagnostic feature set. It also leaves the exact rich-rendering style and multi-file LSP publication architecture undecided.

## Non-goals

This decision does not:

- require rewriting every existing diagnostic before rich rendering ships;
- define the exact visual theme or color palette of terminal output;
- require terminal diagnostics and editor diagnostics to have identical layouts;
- make checker diagnostics depend on the optional LSP semantic `SpanIndex`;
- introduce automatic fixes or editor code actions;
- establish a public stability policy for diagnostic wording;
- require a raw file path to remain the permanent representation of source identity;
- define a rustc-compatible terminal layout.

## Related

- [Rust compiler diagnostic guide](https://rustc-dev-guide.rust-lang.org/diagnostics.html)
- [rustc JSON diagnostics](https://doc.rust-lang.org/rustc/json.html)
- [Gleam diagnostic implementation](https://github.com/gleam-lang/gleam/blob/main/compiler-core/src/diagnostic.rs)
- `docs/adrs/0043-rebuild-lsp-on-snapshot-analysis.md`
- `compiler/checker/checker.go`
- `compiler/lsp/diagnostics.go`
- `compiler/parse/ast.go`
