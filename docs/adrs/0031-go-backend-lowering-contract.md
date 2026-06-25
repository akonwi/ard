# 0031: Go Backend Lowering Contract

## Status

Proposed

## Context

The Go/native backend is the reference target for Ard (see `0002-use-air-as-backend-boundary.md`). It has grown incrementally, and its lowering rules now live mostly in code and accumulated test expectations rather than in a stated contract. This makes the backend hard to evolve and makes it unclear which behaviors are intentional Ard semantics, which are Go-specific conveniences, and which are historical accidents that should be removed.

We want a single durable contract describing how every Ard language feature lowers to Go: packages, modules, entry points, names/visibility, variables, types, functions, methods, traits, control flow, runtime shapes, and FFI. The contract is the target a rewrite (or continued refactor) of the Go backend should converge on, instead of "whatever the current backend happens to do."

This ADR is built section by section. Each section states the lowering rule for one feature area. Where this ADR conflicts with earlier ADRs, this ADR governs Go lowering specifically.

This ADR supersedes the parts of `0002-use-air-as-backend-boundary.md` that say the Go backend assembles "one generated Go package per Ard project" and "one generated Go file per Ard module." Everything else in `0002` (AIR as the backend boundary, preserved runtime shapes, no universal `any`/`runtime.Object`) still holds.

## Guiding principle

Lower Ard to idiomatic Go. Go's own constructs — packages, exported/unexported identifiers, structs, interfaces, methods, pointers — should carry Ard semantics directly wherever they can. Generated synthetic helper types and dispatch machinery are a cost, not a feature: every one we keep is a place where the output stops looking like ordinary Go and stops interoperating naturally with Go code.

The only shared runtime types the backend is allowed to depend on for encoding irreducible Ard semantics are:

- `runtime.Maybe[T]`
- `runtime.Result[T, E]`
- `runtime.Fiber[T]`

Introducing any other generated cross-cutting helper type requires justification that it encodes an Ard semantic Go cannot express directly. Helpers that merely paper over an unfinished lowering should be removed as the lowering is completed.

## Decision

### Packages and modules

#### One Go package per Ard module

Each Ard `.ard` file is a module (`0013-use-file-based-modules-and-absolute-imports.md`). Each Ard module lowers to exactly one Go package.

```text
Ard module (file)  →  Go package (directory)
```

This mapping is what makes Ard visibility lower naturally:

- public Ard declarations → exported Go identifiers
- `private` Ard declarations → unexported Go identifiers, invisible across package boundaries by construction
- identically named declarations in different Ard modules no longer collide, because they live in different Go packages

Because the package boundary enforces visibility and disambiguates names, the backend must not reintroduce module-name-prefixed artifact identifiers (for example `app_models_user__Store`) to avoid collisions. Disambiguation is the package's job.

#### Generated output is a real Go project named after the Ard project

The generated workspace is a complete, buildable Go module.

- The Go module path is the bare Ard project name (from `ard.toml`, or the project root directory name when there is no manifest). Example: `module myapp`.
  - A future extension may allow an explicit import-prefix override (for example `module github.com/user/myapp`) so output is `go get`-able. This is out of scope here.
- Generated import paths mirror Ard module paths exactly, with each path segment sanitized to a valid Go package identifier.

```text
myapp/                       // module myapp
  go.mod
  main.go                    // synthetic entry package (see below)
  accounts/user/user.go      // accounts/user.ard  → package user
  ard/io/io.go               // ard/io             → package io
```

Standard library modules under `ard/*` are ordinary Ard modules and lower under exactly these same rules. They receive no special-case naming. Public stdlib declarations are exported Go identifiers like any other module's.

#### Package naming and sanitization

A module's Go package name is the sanitized basename of its `.ard` file; its Go package directory mirrors the Ard module path with each segment sanitized independently.

Sanitization is deterministic:

- a name that is already a valid Go identifier is kept as-is, including names with underscores such as `foo_bar`
- invalid identifier characters become `_`, with repeated underscores collapsed
- a leading digit is prefixed to form a valid identifier
- a Go keyword gets a trailing `_`
- an otherwise-empty result falls back to a stable placeholder

Only the final filename segment strips the `.ard` extension; directory segments are sanitized without extension stripping, so a directory like `v1.0/` becomes `v1_0`.

Two modules in different directories whose basenames sanitize to the same Go package name are allowed; their import paths differ, and importers disambiguate with import aliases as needed.

`main` is a reserved package name (see entry points). Any Ard module whose sanitized package name would be `main` is remapped to a non-reserved name (for example `main_`), because Go cannot import a `package main`.

#### Entry points are always synthetic

Ard has no concept of a "main module": any module may define an entry. The backend therefore never transpiles an Ard module into `package main`. Instead:

- Every Ard module — including one that defines an entry — lowers to an ordinary importable package under the normal rules. An Ard `fn main` is just a public function and lowers to an exported `Main` in its own package.
- When the program has an entry root (AIR `Entry`) or a top-level-statement script root (AIR `Script`), the backend generates a separate, fully synthetic root `package main` whose only responsibility is to call the entry package's root:

```go
package main

import entry "myapp/app"

func main() {
    entry.Main()
}
```

- For a script root, the synthetic `main` calls the entry module's generated script root function instead.
- A library project with no entry or script root emits no synthetic `main` package; it is just a set of importable packages.

This keeps `package main` fully generated and decoupled from Ard source, so entry selection is a property of lowering rather than of any module's name.

#### Dependencies

An Ard project may depend on other Ard projects (`0017-use-git-based-dependencies.md`). Dependency modules lower into the same generated Go module as the root project, under a namespaced subtree rather than as separate Go modules. This keeps the output a single self-contained Go module and avoids per-dependency `replace` wiring. The dependency subtree is import-path namespaced so dependency module paths cannot collide with the root project's module paths.

#### Host Go code placement

The Go code a project provides is carried into the generated Go module verbatim, as its own ordinary Go package, and imported from Ard via `use go:` like any other Go package (`0028-use-direct-go-imports-for-ffi.md`). It is not inlined into the declaring module's package. This keeps packages shared across modules from being duplicated and keeps package-level state intact. The FFI section describes this and the deprecation of the `extern` binding mechanism in full.

### Names and visibility

#### Visibility mapping

Ard is public by default; `private` makes a declaration module-local. Visibility maps onto Go's exported/unexported distinction:

| Ard declaration | Visibility | Go identifier |
| --- | --- | --- |
| function | public (default) / `private` | exported / unexported func |
| struct, enum, trait, union | public (default) / `private` | exported / unexported type |
| struct field | always public within its type | always exported field |
| inherent method | public (default) / `private` | exported / unexported method |
| trait-implementing method | always public (contract) | always exported method |
| `let` module variable | public | exported var |
| `mut` module variable | private | unexported var |

Because the Go package boundary already enforces visibility, the backend relies on exported/unexported casing for access control and does not add module-name-prefixed identifiers.

#### Identifier conversion

Ard identifiers are converted to Go identifiers mechanically and deterministically:

- split the Ard identifier on `_`
- normalize each segment by lowercasing it, then capitalizing its first letter
- for an exported result, keep the first segment capitalized; for an unexported result, lowercase the first letter of the first segment
- join segments with no separator
- if the result is a Go keyword, append a trailing `_`

The conversion is purely lexical. It applies no Go initialism dictionary: `user_id` becomes `UserId`, not `UserID`. Segment normalization means all-caps and screaming-snake names produce readable camel/Pascal output rather than run-on capitals.

```text
make_user        (public)   → MakeUser
format_name      (private)  → formatName
User             (public)   → User
internal_config  (private)  → internalConfig
api_url          (public)   → ApiUrl
MAX_RETRIES      (public)   → MaxRetries
type             (private)  → type_
```

Field names follow the same conversion and are always exported, regardless of the owning struct's visibility. This keeps every struct serializable by `encoding/json`; the struct type's own visibility still governs cross-package naming.

#### Collisions

Go has a single package-level identifier namespace shared by types, functions, variables, and constants (including enum variant constants). Ard's own scope rules already forbid same-namespace duplicates within a module — two values, or two types, cannot share a name in one file — so the only collision that reaches Go lowering is **type vs value**.

When a generated type name collides with a value identifier in the same package, the type yields and gets `Type` appended:

```ard
struct User {}
fn user() User { ... }
```

```go
type UserType struct{}
func User() UserType { ... }
```

If appending `Type` still collides, a deterministic numeric suffix is applied. This disambiguation is stable across builds.

#### Immutability is not represented in Go

Ard immutability — `let` bindings, immutable struct fields, and the immutability of public `let` globals — is enforced by the checker and is not represented in the generated Go. Exported package variables and exported struct fields are therefore mutable from pure Go code. This is an accepted limitation of idiomatic interop: the backend does not generate getter or accessor wrappers to enforce immutability, because doing so would make the generated API unnatural to use from Go.

#### Locals, parameters, and reserved identifiers

Local variables and parameters never cross package boundaries, so they are converted with the same lexical rules and disambiguated with deterministic suffixes when they would shadow or collide with a generated name, a Go keyword, or a Go predeclared identifier. Reserved Go method names required by interfaces (for example `String`, `Error`, JSON marshaling methods) are handled in the functions/methods and traits sections.

### Types

The backend lowers Ard types to Go types directly. The only shared runtime types it may rely on are `runtime.Maybe[T]`, `runtime.Result[T, E]`, and `runtime.Fiber[T]`; everything else is either a Go builtin, a Go composite, or the generated representation of a specific user type.

#### Primitives

| Ard | Go |
| --- | --- |
| `Int` | `int` |
| `Float` | `float64` |
| `Bool` | `bool` |
| `Byte` | `byte` |
| `Rune` | `rune` |
| `Str` | `string` |
| `Void` | `struct{}` |

`Void` is Ard's unit type. A `Void`-returning function lowers to a Go function with no results. Where `Void` appears as a value or a type argument (`Result[Void, E]`, `Maybe[Void]`, or a generic instantiated at `Void`), it lowers to Go's canonical unit type `struct{}`, with the value `struct{}{}`. The backend does not define a named runtime unit type.

#### Containers

- `List[T]` lowers to `[]T`.
- `Map[K]V` lowers to `map[K]V`. Map key types are constrained, during checking, to types that are Go strictly-comparable and whose Go `==` matches Ard equality. This allows all primitives (including `Float`), enums, and structs whose fields are all valid key types. It excludes `Maybe` (which is Go-comparable but would compare by pointer identity, not value, because it is pointer-backed), `List`, `Map`, and `Dynamic`. Every Ard map therefore lowers to a plain Go map and no structural-map helper is generated. `Float` keys carry Go's usual `NaN`-key behavior, which is accepted.
- `Struct` lowers to a Go struct. Fields appear in AIR index order and are always exported regardless of the struct's visibility, so every struct is uniformly serializable; the struct type's own visibility still governs cross-package naming. Each field carries a `json` tag preserving its original Ard name (see the JSON and marshalling section). Field indirection is not added for the common recursive cases: a nullable self-reference is finite because `Maybe[T]` is pointer-backed, and list/map self-references are finite through the slice/map. Explicit pointer indirection is inserted only for a non-nullable, non-collection recursive cycle that would otherwise be infinitely sized (see `0020-support-recursive-struct-fields-through-indirection.md`).

#### Maybe, Result, Fiber, Dynamic

- `Maybe[T]` lowers to `runtime.Maybe[T]` (`0012-represent-optional-values-with-maybe.md`, `0024-preserve-maybe-semantics-in-go-lowering.md`).
- `Result[T, E]` lowers to `runtime.Result[T, E]` (`0005-use-result-maybe-and-try-for-error-handling.md`). It is not collapsed into Go's `(T, error)` multi-return.
- `Fiber[T]` lowers to `runtime.Fiber[T]` (`0003-use-generic-fibers-for-async-eval.md`).
- `Dynamic` lowers to `any`.

#### Enums

An `enum` lowers to a named `int` type plus typed package-level constants for its variants, using `iota`:

```ard
enum Direction { Up, Down }
```

```go
type Direction int

const (
	DirectionUp Direction = iota
	DirectionDown
)
```

Variant constants are named with the enum's Go type name followed by the PascalCase variant name (`DirectionUp`, `DirectionDown`). The enum type and its variant constants are exported when the enum is public and unexported when it is `private`. Enums backed by a direct Go binding map their variants to the bound Go constants instead of generated `iota` values.

Enums support equality and relational comparisons, including comparisons that mix an enum with `Int`. Because an enum lowers to a named `int` type, a comparison between an enum and an `Int` inserts a Go numeric conversion (`int(d) == code`) so both operands share a type; see the Operators subsection.

#### Unions

A `union` lowers to a generated tagged struct: a discriminant field plus one field per member type. It is not lowered to `any` with a type switch, which would discard type safety and reintroduce universal `any`. The tagged struct is the representation of that specific union type, not a cross-cutting helper. The union type follows the union's visibility, but its discriminant and member fields are always exported so generated match code can read them across packages. A union is not directly JSON-marshallable; see the JSON and marshalling section.

#### Generic type declarations

Generic structs and unions lower to generic Go types. `struct Partition<$T> { selected: [$T], others: [$T] }` lowers to `type Partition[T any] struct { Selected []T; Others []T }`. Trait bounds on type parameters map to Go type constraints and map-key parameters carry a `comparable` constraint, the same as generic functions. Enums are simple named `int` types and are not generic.

#### Type aliases

A union type declaration (`type Name = A | B`) declares a named union and lowers to a named tagged struct, as in the Unions section. Any other type alias lowers to a Go type alias that mirrors the Ard alias, so the intended name appears in the generated API. For example `type Decoder<$T> = fn(Dynamic) $T![Error]` lowers to a generic Go type alias `type Decoder[T any] = func(any) runtime.Result[T, Error]`, and `type Primitive = Str | Bool | Void` follows the named-union lowering. Aliases remain transparent to type checking; the Go alias is for naming, not a distinct type. Parameterized type aliases require Go 1.24 or newer; the compiler and its generated programs target Go 1.26, so this is satisfied.

#### Functions, traits, and extern types

- A function type lowers to a Go `func(params) result`. A mutable (`mut`) parameter lowers to a pointer parameter so the callee can write back through it.
- A value typed as a trait object lowers to the trait's generated Go interface; the interface itself is defined by the trait's module (see the Traits section).
- An extern or direct Go type lowers to the bound Go type or its FFI companion type (`0028-use-direct-go-imports-for-ffi.md`, `0030-use-direct-go-struct-values-and-fields.md`).

### Variables and globals

#### Locals

Both `let` (immutable) and `mut` (mutable) locals lower to ordinary Go variables. Go has no const-local concept, so immutability is a checker-enforced property and is not represented in the generated code. The backend prefers `x := e` and uses `var x T = e` only when a type annotation is required, such as empty container literals or `Maybe`/`Result` zero values.

Local and parameter names use the identifier conversion from the Names section and receive deterministic suffixes when they would shadow another binding or collide with a Go keyword or predeclared identifier. Locals never cross a package boundary, so their casing is irrelevant to visibility.

Generated code must never produce Go "declared and not used" errors. An unused lowered value is discarded with `_ = expr`, and imports are emitted only when used.

#### Module-level globals

Module-level variables lower to package-level Go `var` declarations (`0021-represent-module-level-lets-as-air-globals.md`):

- a module-level `let` is public and lowers to an exported package var
- a module-level `mut` is private module state and lowers to an unexported package var

Module-level variables always lower to `var`, never `const`, even when the initializer is a literal. Ard module lets can hold arbitrary expressions, and uniform `var` lowering avoids special-casing; a future optimization may emit `const` for literal-initialized lets but is out of scope here.

When an initializer requires setup statements (for example a block or match expression), the variable is declared without an initializer and assigned in a generated `init()` function, rather than wrapped in an immediately-invoked function literal.

Initialization order relies on Go's package-level variable initialization: declaration order is preserved for otherwise-independent variables, and reference dependencies are resolved automatically. This matches Ard's top-to-bottom module-load order. Cyclic global initializers are rejected during AIR lowering before code generation.

A `mut` global receives no special treatment beyond being an unexported package variable; it is ordinary package state, as in Ard today.

### Functions and methods

#### Top-level functions

An Ard `fn` lowers to a Go `func`, exported or unexported per visibility. A `Void` return lowers to a Go function with no results; otherwise the function has exactly one Go result, since Ard functions return a single value (which may itself be a `Result` or `Maybe`).

Named and labeled arguments, optional (`Maybe`) arguments, and default argument values are resolved by the checker into positional AIR arguments, with defaults materialized, so the backend emits plain positional Go calls.

#### Impl methods

An impl method lowers to a real Go method on the receiver type. The backend does not also emit a standalone helper function for the method; method calls dispatch through the Go method. A method call `value.method()` lowers to `value.Method()`. Method visibility follows the same rules as functions: public methods are exported, `private` methods are unexported.

#### Receivers

The receiver shape is determined by whether the method mutates `self`:

- a non-mutating method lowers to a value receiver, `func (u User) Method()`
- a mutating method lowers to a pointer receiver, `func (u *User) Method()`

Pointer receivers are how the backend expresses Ard mutation through methods. This replaces the previous mutable-trait forwarding-table representation: a trait whose implementation mutates `self` is satisfied by the pointer type, and call sites pass an addressable value. The addressability requirements this places on trait satisfaction are covered in the Traits section.

#### Mutable parameters

A `mut` parameter lowers to a pointer parameter, and call sites pass the address of an addressable argument. Writeback to the caller's value happens through the pointer. This is the same mechanism as mutating receivers.

#### Closures

Closures lower to Go function literals with ordinary lexical capture. Go function literals capture variables by reference, which matches Ard's mutable-capture semantics, so no separate capture-parameter helper function is generated. The only exception is a closure that must be a named function for its own definition, such as a directly recursive local function; such cases may lower to a named local function rather than an inline literal.

A reference to a top-level function as a value lowers to the Go function identifier; a reference to a method as a value lowers to a Go method value or method expression.

#### Generics

Ard generic functions and methods lower to Go generics: a generic Ard function becomes a single generic Go function with its natural name, and Ard trait bounds map to Go type constraints (interfaces). This keeps the public Go API natural and the output small, rather than emitting many mangled monomorphized specializations.

A method on a generic struct lowers to a real Go generic-receiver method (`func (self Foo[T]) M(...)`), where the receiver binds the struct's type parameters. Because Go method receivers cannot introduce *additional* type parameters, a method that declares its own generic parameters beyond the struct's keeps the monomorphization fallback.

Where a generic cannot be expressed with Go type constraints, the backend falls back to monomorphization, emitting a concrete specialized Go function per instantiation. Monomorphization is the fallback, not the default.

A type parameter used as a map key emits a Go `comparable` constraint, since `map[K]V` requires it. When such a parameter also has a trait bound, the emitted constraint is an interface embedding both `comparable` and the trait's interface. The checker infers the comparability requirement from map-key usage and enforces it at instantiation using the same map-key rule as concrete maps; in particular it is stricter than Go `comparable` in one case, rejecting a `Maybe` type argument for a key parameter even though Go would accept it. On the monomorphization fallback path the key type is concrete and already checked, so no constraint is emitted.

#### Reserved Go method names

The backend does not let generated methods accidentally satisfy Go interfaces with semantic meaning, such as `MarshalJSON`/`UnmarshalJSON` or `error`'s `Error() string`, unless that is intended. A generated method whose natural Go name would collide with such a reserved name is renamed or suppressed so it does not silently change how the type interacts with the Go runtime and standard library. Whether Ard's `ToString` deliberately lowers to Go's `fmt.Stringer` `String() string` is decided in the Traits section.

### Traits

#### Traits are Go interfaces

An Ard trait (`0009-support-traits-for-shared-behavior.md`) lowers to a Go interface defined in the trait's own module package. The interface's methods are the trait's method signatures using natural Go names. A public trait lowers to an exported interface; a `private` trait lowers to an unexported interface.

```ard
trait Drawable { fn draw() Str }
```

```go
type Drawable interface { Draw() string }
```

#### Implementations are structural

`impl Drawable for Button` gives `Button` a `Draw` method, lowered as a real Go method per the Functions section. Go satisfies interfaces structurally, so there is no implementation registration, no generated type-switch dispatch, and no generated trait-dispatch or forwarding-table types. A value typed as a trait object is an ordinary Go interface value, and method calls on it use Go interface dispatch.

A trait definition is entirely a public contract; trait methods have no private form. A method that implements a trait method is therefore always public and lowers to an exported Go method, so it satisfies the trait's interface across packages. Marking a trait-implementing method `private` is rejected by the checker, because a private method cannot satisfy the public contract. The `private` modifier applies only to inherent (non-trait) impl methods, which follow the normal visibility rules. The trait's interface type is exported or unexported according to the trait's own visibility, but its methods are always exported.

This supersedes, for Go lowering, the mutable-trait forwarding-table representation in `0023-represent-mutable-trait-references-with-forwarding-tables.md`. Forwarding tables and generated `ardTrait_*`/`ardMutTrait_*` types are not emitted.

#### Mutation through pointer receivers

Mutability is a property of an implementation, not of the trait: a trait declares required methods, and an `impl` decides whether its implementation mutates `self`. An impl method that mutates `self` lowers to a pointer-receiver method, so that implementer satisfies the trait as `*T`. When such a value is used as a trait object, it is held as a pointer, and the checker must guarantee the upcast value is addressable. Each implementer independently satisfies the interface as `T` or `*T` according to its own methods.

#### Generic trait bounds

A generic bounded by a trait lowers to a Go type parameter constrained by the trait's interface. Where the bound is satisfied by a builtin primitive, which cannot carry Go methods, the Go constraint cannot express it and the generic falls back to monomorphization (see the Functions section), lowering the primitive's trait methods directly.

#### Traits implemented for primitives

Enums, structs, and unions are defined Go types and can carry methods, so they satisfy trait interfaces directly. Builtin primitives (`int`, `string`, and the other primitive lowerings) cannot have Go methods, so traits implemented for primitives are handled in two ways:

- static or generic trait dispatch on a known primitive lowers the trait method directly, with no interface value
- a dynamic trait object whose value is a primitive is boxed in a minimal generated per-primitive adapter type that forwards the trait method

The per-primitive adapter is the one sanctioned synthetic type here, justified solely because Go cannot attach methods to builtins.

#### ToString maps to fmt.Stringer

The prelude `ToString` trait is the one trait given a well-known Go mapping: it lowers to Go's `fmt.Stringer`, and its `to_str` method lowers to `String() string`. A type implementing `ToString` therefore implements `fmt.Stringer` and prints correctly through `fmt`, `io::print`, and ordinary Go code, and a `ToString` trait object is a `fmt.Stringer`. `ToString` is currently the only standard library trait, so it is the only such mapping; user traits receive no special Go-interface mapping.

### Control flow

Ard is expression-oriented and Go is statement-oriented, so the general rule is that a control-flow construct in value position lowers to Go statements that compute into a temporary, and the temporary is used by the surrounding expression. In statement position it lowers to the natural Go statement.

Statement hoisting always suffices; the backend does not use immediately-invoked function literals. Two positions need slightly more care than a plain hoist:

- a package-level initializer that requires statements is assigned in a generated `init()` function rather than inline
- a short-circuit operand (the right side of `&&` or `||`) that requires statements is lowered with a guarded temporary, so its statements run only when the operand is actually evaluated

#### Blocks, if, and match

- A block evaluates to its final expression: its statements lower in order and the final expression becomes the block's value.
- `if`/`else` lowers to Go `if`/`else`; in value position each branch assigns the same temporary.
- `match` lowers per subject:
  - a value-binding match arm binds the matched value into the case body: the implicit `it` binds the subject; named arms such as `ok(x)`, `err(e)`, and `some(x)` bind the extracted `Result`/`Maybe` payload; and a union member arm written as `Type(x)` (for example `Str(s)`, `Int(i)`) binds the matched member field into `x`
  - an enum match lowers to a `switch` on the int value, one case per variant
  - an int match lowers to a `switch`, with range cases lowered to case lists or if-chains
  - a string match lowers to a `switch` on the string
  - a union match lowers to a `switch` on the tag field, extracting the matched member field
  - a `Maybe` match lowers to an `if` on `IsSome`/`IsNone`
  - a `Result` match lowers to an `if` on `Ok`

Match exhaustiveness is guaranteed by the checker. A value-producing match emits an unreachable `panic` default so Go's flow analysis accepts the generated switch.

#### Loops

- `while` lowers to `for cond { }`.
- `for`-in lowers to Go `for range`:
  - over a list, `for _, x := range slice`; with an index binding, `for v, i in list` lowers to `for i, v := range slice` following Ard's value-first binding order
  - over a map, Go's native `for k, v := range m`. Map iteration order is unspecified; the backend does not impose a deterministic order and emits no key-sorting helper.
  - over a string, `for _, r := range s`, which yields runes (the byte offset is discarded); with an index binding, the string is ranged as `[]rune(s)` so the index is a zero-based rune position rather than a byte offset, matching list index semantics
  - over a numeric range or number, a counted `for`
- A for-loop expression (`0011-for-loop-expressions.md`) lowers by building a slice: each iteration appends the body result, and the slice is the loop's value.
- `break` lowers to Go `break`.

#### try and early propagation

`try` provides early-return semantics for `Result` and `Maybe` values. It has a propagating form and a handler form written with `->`; there is no `catch` keyword.

- Propagating `try expr` evaluates the operand into a temporary. For a `T!E` value it lowers to `if !t.Ok { return runtime.Err[...](t.Err) }` and then uses `t.Value`; for a `Maybe` value it lowers to an early `return` of `None` and then uses the unwrapped value. The enclosing function must return a compatible `Result` or `Maybe`, which the checker enforces, with the same error type for `Result`.
- The handler form `try expr -> err { ... }`, or `try expr -> _ { ... }` to ignore the failure value, unwraps and continues on success. On failure it binds the error or none, runs the handler block, and returns the handler block's value early from the enclosing function; the handler block may also `return` explicitly. It lowers to `if !t.Ok { err := t.Err; return <handler> }` for `Result`, and analogously for the `Maybe` none case.
- The function-reference form `try expr -> fnRef` is shorthand for a handler that passes the error to `fnRef` for `Result`, or calls `fnRef` for the `Maybe` none case, returning its result early.
- A `try` over a chain of `Maybe` property accesses, such as `try a.b.c -> ...`, desugars to nested `Maybe` matches that short-circuit to the handler (or propagate early) on the first `None`, rather than a single unwrap of one temporary.
- `panic`, `unreachable`, and an `expect` on an absent `Maybe` or an error `Result` lower to Go `panic`. Assertion helpers such as `testing::assert` are ordinary `Void!Str` function calls, not panics.

#### Operators

- Arithmetic and comparison operators on primitives lower to the corresponding Go operators. The keyword operators `and`, `or`, and `not` lower to `&&`, `||`, and `!`. Compound assignment (`=+` and the other compound forms) lowers to the corresponding Go compound assignment (`+=`, etc.).
- Equality (`==`/`!=`) and relational comparisons (`<`, `>`, `<=`, `>=`) are supported on primitives, nullable primitives, and enums, including comparisons that mix an enum with `Int`. Primitive comparisons lower to the corresponding Go operators.
- Nullable-primitive equality lowers to an inline presence-and-value comparison, because `runtime.Maybe` is pointer-backed and cannot use Go `==`. There is no structural equality over structs, lists, or maps, so no general equality helper is generated.
- Enum comparisons lower to Go comparisons on the enum's named `int` type. When an enum is compared with an `Int`, the backend inserts a Go numeric conversion so both operands share a type, converting the enum operand to `int` (`int(d) == code`), since Go does not allow comparing a named int type directly against `int`.
- Chained relative comparisons (`0006-support-chained-relative-comparisons.md`) lower to conjunctions, so `a < b < c` becomes `a < b && b < c`.

### Builtins, literals, and conversions

Builtin operations on primitives and collections lower to Go builtins and the Go standard library rather than to runtime helpers. The contract states the rule rather than enumerating every method.

#### Literals and interpolation

- Byte and rune literals lower to Go `byte` and rune literals, including escapes such as `'\n'`, `'\x00'`, and `'\u0080'`.
- String interpolation lowers to string concatenation of the literal segments and each interpolated expression's `to_str` lowering. Primitive interpolations use direct conversion (for example `strconv`), and `ToString` values use their `String()` method, so interpolation does not depend on `fmt` reflection.

#### Builtin collection and string operations

Builtin collection and string operations lower to Go builtins and the `strings`, `strconv`, and `unicode/utf8` packages. Representative cases: `.size()` to `len`, list `.push` to `append`, list and string `.at` to a bounds-checked `Maybe`, map `.get` to a comma-ok `Maybe`, map `.set` to assignment, `.contains` to a comma-ok check, `.bytes()`/`.runes()` to `[]byte`/`[]rune` conversions, and `.split`/`.starts_with`/`.ends_with`/`.replace`/`.trim` to the corresponding `strings` functions. Operations that can fail a bounds or parse check produce a `Maybe` or `Result`.

#### Numeric and primitive conversions

Conversions such as `to_int`, `to_float`, `to_str`, `Byte::from_int`, `Rune::from_int`/`from_str`, and `Str::from_bytes`/`from_runes` lower to Go conversions and `strconv` calls. Conversions that can fail a range or parse check produce a `Maybe` or `Result`. Mixing `Byte`, `Int`, and `Rune` inserts the explicit Go numeric conversions Go requires between distinct numeric types.

### Runtime shapes

The generated program depends on a single small runtime package, `github.com/akonwi/ard/runtime`. That package exists only to hold the shapes that encode Ard semantics Go cannot express directly. Everything else lives in generated code, Go builtins, or standard library FFI companions.

#### Sanctioned runtime types

The runtime defines exactly three types:

- `runtime.Maybe[T]` for optional values. It is pointer-backed internally, which is intentional: the indirection makes recursive nullable fields finite-size, and it is the reason nullable-primitive equality lowers inline and `Maybe`-keyed maps are disallowed.
- `runtime.Result[T, E]` for fallible values, with `Value`, `Err`, and `Ok` fields that `try` reads directly. It is not collapsed into Go's `(T, error)`.
- `runtime.Fiber[T]` for asynchronous results, with `SpawnFiber`, `JoinFiber`, and `GetFiber`.

No other shared runtime type is introduced. The one exception sanctioned elsewhere in this contract is the minimal per-primitive boxing adapter required for dynamic trait objects over builtins, which is generated, not part of the runtime package.

#### Async

Fiber-based async (`0003-use-generic-fibers-for-async-eval.md`) uses `runtime.Fiber` and its spawn/join/get functions. Typed channels (`0019-use-typed-channels-for-fiber-communication.md`) lower to native Go `chan T` rather than a runtime type.

#### Dynamic

`Dynamic` lowers to `any`. The runtime contains no universal boxed object or kind-tagged value representation. Operations on dynamic data are provided by the relevant standard library modules and their FFI companions, not by a runtime object model.

#### Leaf-dependency rule

The runtime package may import only the Go standard library. It must never import `checker`, `air`, or any other compiler package, because generated programs depend on the runtime and must not pull in the compiler. Any runtime code that depends on compiler packages is by definition not a runtime shape and does not belong here.

#### Standard library boundary

Standard library behavior lives in Ard modules and their FFI companions under `std_lib/ffi`, not in the runtime package. Utilities that back specific stdlib modules — for example command-line argument access for `ard/argv` — are provided through FFI companions rather than the runtime, keeping the runtime limited to the three semantic types and their async/try support.

### JSON and marshalling

Ard JSON support lowers onto Go's `encoding/json/v2` over the generated Go types, rather than a custom typed codec or a boxed object model. Typed JSON (`ard/json`) uses `Marshal`/`Unmarshal` directly, and `Dynamic`-based decoding (`ard/decode`) operates on `any`. The previous universal `runtime.Object`/`Kind` representation is removed.

#### What is marshallable

Marshalling applies to every JSON-representable type: `Dynamic`, structs, lists, string-keyed maps, primitives, `Maybe`, `Result`, enums, and unions. `Dynamic` is the general-purpose marshalling vehicle. A union marshals to its active member's value, unwrapped, so a `Str | Int` holding `"hi"` marshals to `"hi"` and one holding `5` marshals to `5`; the union tag is not part of the wire form.

Unmarshalling (`json::parse`) applies to the same shapes with one exception: parsing into a union is not supported, because choosing a member from arbitrary JSON is ambiguous. The checker rejects `json::parse` into a union or a type that contains one.

The `ard/encode` module, including the `Encodable` trait and its `to_dyn` method, is also removed. It existed to give a single customizable API for turning a value into a particular encoded form, but that capability is unused in practice; building a `Dynamic` goes through the `Dynamic` constructors instead. This keeps `ToString` as the only standard library trait.

#### Struct fields

Struct fields are always exported and each carries a `json:"<original_ard_field_name>"` tag, so the wire format uses the Ard field name even though the Go identifier is converted (`name` to `Name`, `user_id` to `UserId`). Because fields are always exported, every struct is serializable regardless of its type's visibility.

#### Maybe

`runtime.Maybe[T]` implements JSON marshalling and unmarshalling: `none` marshals to `null`, `some(x)` marshals to `x` unwrapped, a present value unmarshals to `some`, and a JSON `null` or a missing field unmarshals to `none`. `Maybe` fields are not `omitempty`, so `none` is emitted as `null`.

`runtime.Result[T, E]` also implements JSON marshalling: `ok(x)` marshals to `x` and `err(e)` marshals to `e`, each unwrapped. `Maybe` and `Result` are the two sanctioned exceptions to the reserved-method rule, because they are runtime types the backend owns and their JSON shapes encode Ard semantics directly.

#### Enums

An enum marshals as its underlying integer value, which a named `int` type does natively, so no marshaller is generated. Enums backed by a direct Go binding marshal as their bound values.

#### Lists, maps, and bytes

Lists lower to `[]T` and maps to `map[K]V`, both marshalled natively. A `[Byte]` value lowers to `[]byte`, which `encoding/json` encodes and decodes as base64, matching Ard's byte-buffer JSON behavior.

#### Dynamic and number disambiguation

`Dynamic` lowers to `any`, and JSON parses into the ordinary Go shapes `map[string]any`, `[]any`, `string`, `bool`, and `nil`. Numbers decoded into `Dynamic` use `json.Number` so that `decode` combinators can distinguish `Int` from `Float`; typed parsing into a known type decodes numbers directly into the target `Int` or `Float` field with no ambiguity. The runtime holds no boxed object or kind-tagged representation for dynamic data.

`Dynamic` is kept, but demoted. As a boxed runtime representation it is gone; it is now simply the named Ard surface for Go's `any`, with no runtime machinery. It remains the substrate for genuinely untyped data — `ard/decode`, schema-unknown or partial JSON, SQL rows, and host values typed as `any` — but it is no longer the default path for structured data, because `json::parse<T>` decodes directly into generated structs. How prominent `Dynamic` stays depends on a separate, future language decision about whether to keep `ard/decode` and whether SQL moves to typed row scanning; if those untyped producers move to typed paths, `Dynamic` becomes a rarely-used escape hatch. That decision is out of scope for this ADR, which only fixes that `Dynamic` lowers to `any`.

### Tests

Ard test functions (`0014-use-ard-native-test-functions.md`) lower under the same module-to-package rules as ordinary code, with a synthetic runner driving them.

#### Test functions

A `test fn` lowers to an ordinary function in its module's package and is included only in test builds; production builds omit test functions entirely. A test returns `Void!Str`, which lowers to `runtime.Result[struct{}, string]`. Assertion helpers from `ard/testing` return `Void!Str` and compose with `try`.

The visibility split from `0014` is enforced for free by Go's package boundary:

- a co-located test shares its module's package and can therefore call that module's unexported (`private`) symbols
- a test in the `/test` directory is a separate module and therefore a separate package, so it can reach only the public API of the modules it imports

#### Test runner

The runner is a synthetic `package main`, analogous to the entry main but for tests. It gathers every test, invokes each with `recover()` for panics, interprets the returned `Result[struct{}, string]` (`Ok` is a pass, `Err` is a failure with its message, a panic is an errored test), and reports outcomes.

To reach tests across packages without exporting arbitrary test functions, each test-bearing package emits a single exported aggregator, for example `func ArdTests() []ardTest`, whose entries pair each test's name with a thunk referencing the package's own test functions. The thunks may reference unexported test functions because they are defined in the same package. The runner imports each test package and calls its aggregator, so individual test functions stay unexported and no registry or `init()` side effects are needed.

### FFI and direct Go interop

Interop with Go is a single mechanism: direct Go interop through `use go:`. There is no separate foreign-function binding layer in the target model. Calling Go from Ard is calling Go directly.

#### Direct Go interop is the one mechanism

`use go:` imports a real Go package and Ard calls it directly (`0028-use-direct-go-imports-for-ffi.md`, `0030-use-direct-go-struct-values-and-fields.md`):

- `use go:image as image` lowers to a Go import, and `image::Point{...}` lowers to `image.Point{...}`
- struct values, field access, and method calls go straight through to the Go package
- because Ard impl methods now lower to natural Go methods, an Ard type satisfies a Go interface structurally, with no generated wrapper layer

#### Host Go code is a package in the output

The Go code a project provides is carried into the generated Go module verbatim, as an ordinary Go package, and is imported from Ard with the go prefix like any other Go package. There is no companion-rewriting step, no per-module copying, and no special package clause handling.

- A project's Go package (for example `package ffi`) is copied untouched into the generated module at its mirrored path, such as `<project>/ffi`.
- Ard references it with `use go:<project>/ffi`, and calls it exactly as it would call `math` or `image`.
- Dependency Go packages are carried in the same way, under the dependency's namespaced subtree.

This supersedes the earlier statement in the Packages and Modules section that FFI companions land in the declaring module's package. Host Go code is its own package and is imported, not inlined, so packages shared across modules are not duplicated and package-level state is not split.

#### Deprecating extern

The `extern fn` and `extern type` declarations, and the `extern` keyword, exist only to bind Ard declarations to host functions and types. Direct Go interop makes them redundant: anything an extern expressed is expressed by importing the Go package and calling it. The `extern` mechanism is therefore deprecated and is removed once migration is complete.

The standard library participates in this migration. Today `std_lib/*.ard` uses `extern fn` extensively; under this model the standard library's Go implementations are an ordinary Go package that the `ard/*` modules import via `use go:`, exactly like user-provided Go code. Until the migration finishes, `extern` lowering remains supported as a direct call to its bound Go target, with the required import added.

## Consequences

- The generated workspace is a real, buildable Go module named after the Ard project, with one package per Ard module mirroring Ard module paths.
- Ard visibility maps onto Go's package/export boundary, so cross-module name collisions are resolved structurally and module-name-prefixed artifact identifiers are no longer needed.
- `package main` is always synthetic and decoupled from Ard source, matching Ard's model where any module can host an entry.
- The standard library lowers under the same rules as user code, which requires stdlib public declarations to be exported with natural Go names.
- Interop with Go is unified into direct `use go:` interop. Host Go code is carried into the generated module as an ordinary imported package, the `extern` binding mechanism is deprecated and eventually removed, and the standard library migrates from `extern fn` to importing its Go implementation package via `use go:`.
- Dependencies remain inside one Go module under a namespaced subtree, keeping output self-contained.
- The backend commits to minimizing synthetic helpers, with only `Maybe`, `Result`, and `Fiber` sanctioned as shared runtime types. `Void` lowers to `struct{}` and the structural-map helper is removed.
- Equality is restricted to primitives, nullable primitives, and enums, so no structural-equality helper is generated. Map iteration order is unspecified and uses Go's native range, removing the sorted-key helpers. The previously generated equality and key-sorting helpers become dead and are removed.
- The runtime package is reduced to `Maybe`, `Result`, and `Fiber` and must import only the Go standard library. The universal boxed `Object`/`Kind` representation, structural-map, equality, sorted-key, and void helpers are removed, and stdlib utilities such as argv access move to FFI companions.
- JSON lowers onto Go's `encoding/json/v2` over the generated types. Struct fields are always exported (revising the earlier fields-follow-type-visibility rule) and carry `json` tags preserving the Ard field name, so every struct is serializable. Marshalling applies to `Dynamic`, structs, lists, string-keyed maps, primitives, `Maybe`, `Result`, enums, and unions (a union marshals to its active member, unwrapped); only parsing into a union is rejected by the checker, as it is ambiguous. `Maybe` and `Result` are the sanctioned runtime JSON marshalers. Decoding into `Dynamic` uses `json.Number` to disambiguate `Int` from `Float`.
- Map key types are constrained during checking to Go strictly-comparable types whose Go equality matches Ard equality, so every Ard map lowers to a plain Go map. `Float` keys are allowed; `Maybe`, `List`, `Map`, and `Dynamic` keys are rejected. Generic type parameters used as map keys emit a Go `comparable` constraint. This is a language-level constraint the checker must enforce. The standard library conforms to it: `ard/decode`'s `to_map` is removed and decode migrates to string-keyed maps, which is the correct shape for JSON objects anyway.
- Impl methods lower to real Go methods with no duplicated standalone helpers, mutation is expressed through pointer receivers and pointer parameters rather than forwarding tables, and closures lower to Go function literals. Generic functions lower to Go generics, with monomorphization only as a fallback. These choices require AIR to preserve generic structure and receiver/mutation metadata for the backend.
- Traits lower to Go interfaces with structural satisfaction, removing generated dispatch and forwarding-table machinery and superseding the Go-lowering portion of `0023-represent-mutable-trait-references-with-forwarding-tables.md`. `ToString` maps to `fmt.Stringer`. Traits implemented for builtin primitives require a minimal per-primitive boxing adapter for dynamic trait objects, since Go cannot attach methods to builtins.
- Trait methods are always public contract, so trait-implementing methods always lower to exported Go methods. The checker must reject a `private` modifier on a trait-implementing method.
- Remaining sections of this contract still need to be drafted before the contract is complete enough to drive a backend rewrite.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0006-support-chained-relative-comparisons.md`
- `docs/adrs/0011-for-loop-expressions.md`
- `docs/adrs/0003-use-generic-fibers-for-async-eval.md`
- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
- `docs/adrs/0014-use-ard-native-test-functions.md`
- `docs/adrs/0017-use-git-based-dependencies.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
- `docs/adrs/0030-use-direct-go-struct-values-and-fields.md`
- `docs/adrs/0016-defer-project-ffi-codegen.md`
- `docs/go-backend-idiomatic-lowering.md`
