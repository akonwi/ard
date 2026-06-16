# 0028: Use Direct Go Imports for FFI

## Status

Proposed

## Context

ADR 0029 removed Ard's JavaScript targets, leaving Go as the active execution backend. That gives FFI work a cleaner direction: optimize for Ard code that calls Go code directly, instead of preserving target-aware abstractions for an experimental backend.

The current Go FFI model still asks users to mirror Go APIs through Ard `extern` declarations plus project-local `ffi.go` or `ffi/*.go` companion files. Companion code is useful when Ard needs an adapter, but it is too much ceremony when Ard only needs to call exported Go functions, refer to exported Go types, or use exported typed constants.

Go libraries also commonly model enums as named integer types plus exported typed constants:

```go
type AnimationStatus int

const (
	AnimationIdle AnimationStatus = iota
	AnimationForward
	AnimationCompleted
)
```

Today Ard code must mirror that shape manually, often by declaring an Ard enum and writing conversion helpers. That duplicates source of truth and is easy to drift from the Go package.

The standard library is a useful first proving ground. Many current Go standard-library bindings in `compiler/std_lib/ffi/host.go` are thin forwarding functions that exist mostly because Ard cannot name Go packages directly. Others are real adapters that translate Go conventions into Ard semantics and should remain as companion code.

## Decision

Add direct Go imports to Ard source using `use go:` syntax:

```ard
use go:git.sr.ht/~rockorager/vaxis as vaxis
```

A Go import introduces a Go namespace, not an Ard module. Exported Go symbols are referenced with Ard namespace syntax:

```ard
vaxis::Vaxis
vaxis::New
vaxis::AnimationIdle
```

Go type references use Ard type syntax. A named Go type can appear directly in Ard signatures without a separate `extern type` declaration:

```ard
extern fn new() mut vaxis::Vaxis!Str = vaxis::New
extern fn close(vx: mut vaxis::Vaxis) Void!Str = vaxis::Vaxis::Close
```

Raw Go pointer syntax should not appear in Ard source. Go pointer types are represented with Ard's mutable-reference syntax:

- `vaxis::Vaxis` lowers to `vaxis.Vaxis`
- `mut vaxis::Vaxis` lowers to `*vaxis.Vaxis`

Direct Go extern bindings should use Ard namespace syntax instead of stringly qualified Go names. Existing string bindings and companion FFI remain supported for compatibility and adapter code.

A project that uses `use go:` relies on the project's own `go.mod`. Do not add Ard-specific `go.dependencies` metadata. The compiler should load Go package metadata in the project module context and report actionable diagnostics when a package is missing from `go.mod` or cannot be loaded.

Use `golang.org/x/tools/go/packages` as the compiler-facing Go metadata loader. Load packages with the Ard project root as `packages.Config.Dir` so ordinary Go module, workspace, replace, build tag, and dependency behavior applies. Use readonly module loading initially so Ard does not mutate `go.mod`; missing packages should produce diagnostics that tell the user to run `go get` or `go mod tidy` themselves.

Do not depend on `gopls` for compiler semantics, and do not manually infer Go package types from parsed ASTs. The checker should depend on a small internal Go package resolver abstraction backed by `go/packages`; Ard's LSP may reuse that resolver later for completions and hovers, but the compiler should remain deterministic and library-driven.

### Boundary scalar coercions

Direct Go bindings still need Ard-to-Go boundary adaptation. The checker should validate extern declarations against the loaded Go signature using Ard boundary compatibility, not exact Go type identity. The Go backend should then emit explicit conversions and checks at the call boundary.

Primitive and scalar values should be coerced automatically where the mapping is well-defined:

- `Bool` ↔ `bool` and named Go boolean types
- `Str` ↔ `string` and named Go string types
- `Int` ↔ Go integer types and named Go integer types
- `Byte` ↔ `byte`, `uint8`, and named Go byte-like types
- `Rune` ↔ `rune`, `int32`, and named Go rune-like types
- `Float` ↔ `float32`, `float64`, and named Go floating-point types

For Ard-to-Go calls, generated code should insert conversions such as `time.Duration(value)` or `uint32(value)` when a Go function expects a named or width-specific scalar. Conversions that can fail because of range or sign, such as `Int` to `uint8`, should be checked before conversion rather than silently wrapping. If the failure cannot be represented in the declared Ard return type, the boundary should fail loudly.

For Go-to-Ard returns, generated code should convert scalar results back to Ard primitive representations and validate ranges where needed. For example, a Go `uint8` can return as `Byte`, and a Go `int32` can return as `Rune` only if it is a valid Unicode scalar value.

Named Go scalar types do not need to be surfaced as separate Ard types when the Ard API only wants a primitive. For example, a direct binding may expose `time.Sleep(time.Duration)` as an Ard function taking `Int` while the generated boundary converts the `Int` to `time.Duration`.

### Go enum-like constants

Treat exported typed Go constants as Ard enum-like values when they have an exported named integer type and are used through the Go namespace. The checker should discover exported constants from imported Go packages and expose them directly:

```ard
use go:git.sr.ht/~rockorager/vaxis as vaxis

fn active(status: vaxis::AnimationStatus) Bool {
  match status {
    vaxis::AnimationIdle => false
    vaxis::AnimationForward => true
    vaxis::AnimationCompleted => false
  }
}
```

Do not require or add an `extern enum` declaration. Do not strip prefixes or rename constants. The Ard names are the exported Go constant names exactly as imported through the Go namespace.

Within Ard, a Go enum-like type is treated as closed over the discovered exported typed constants. Exhaustiveness checking may use that discovered set. To preserve Ard's closed-enum semantics at the FFI boundary, values of imported enum-like types returned from Go extern calls should be validated before entering ordinary Ard code. If Go returns a value outside the discovered set, the generated Go boundary code should fail loudly rather than manufacture an impossible Ard enum value.

Go permits multiple exported typed constants to have the same value. Ard should import those constants as aliases rather than rejecting the package. Exhaustiveness for imported enum-like types is based on the set of distinct constant values, so matching any alias covers that value. If a `match` includes multiple aliases for the same value, later arms for that value are unreachable and should be diagnosed as unreachable or duplicate patterns.

Do not add `extern const` as part of this decision. Direct imported Go constants are only promoted into Ard values through the enum-like typed constant rule above.

### Standard-library migration

Apply direct Go imports to the standard library where the binding is truly direct. Examples include:

```ard
use go:math as math
use go:strings as strings
use go:encoding/hex as hex

extern fn floor(float: Float) Float = math::Floor
extern fn ceil(float: Float) Float = math::Ceil
extern fn round(float: Float) Float = math::Round
extern fn split(input: Str, delimiter: Str) [Str] = strings::Split
extern fn encode(bytes: [Byte]) Str = hex::EncodeToString
```

Go-backed stdlib handle types can also become direct Go type references instead of stringly `extern type` declarations:

```ard
use go:database/sql as sql
use go:net/http as http

private type Db = mut sql::DB
private type Tx = mut sql::Tx
private type RawRequest = mut http::Request
private type RawResponse = mut http::Response
```

Keep companion FFI wrappers where Ard's public API intentionally differs from the Go function shape. Direct imports do not replace adapters that need to:

- choose defaults or add missing Go arguments, such as file permissions, open flags, timeouts, hash costs, or scrypt parameters;
- convert Go `(T, error)` or `(T, bool)` conventions into Ard `Result` or `Maybe` semantics beyond the basic boundary adapters;
- normalize values, such as SQL placeholders, password text, dynamic values, JSON values, HTTP request/response structs, or file-system entry maps;
- manage state, such as buffered stdin or configured host arguments;
- adapt callbacks and handlers between Ard function types and Go callback signatures;
- access Go variables such as `os.Args`, since this decision does not add general constant or variable binding.

This means direct Go imports reduce stdlib FFI ceremony but do not eliminate `compiler/std_lib/ffi` entirely. The remaining companion code should become clearer because it represents real semantic adaptation rather than mandatory forwarding.

The standard-library migration exposes implementation gaps that should be addressed before broad conversion:

- method-expression bindings for exported Go methods, such as `sql::DB::Close` or `http::Request::PathValue`;
- boundary adapters for `error -> Void!Str`, `(T, error) -> T!Str`, `(T, error) -> T?`, and `(T, bool) -> T?`;
- boundary scalar coercions for direct Go signatures whose scalar types do not exactly match Ard's primitive Go representations;
- a way to represent direct Go packages used by embedded stdlib code with compiler-module metadata rather than a user project `go.mod`;
- clear diagnostics when a direct Go function's required parameters cannot be expressed by the Ard extern signature.

## Consequences

- Ard code can call Go packages more directly without always writing project companion FFI adapters.
- Existing `ffi.go` and `ffi/*.go` companions remain important for adapting APIs whose signatures do not map cleanly into Ard.
- Ard source keeps Ard syntax for names and types, even when the referenced symbol comes from Go.
- Go pointer interop follows Ard's `mut` reference model rather than exposing Go pointer syntax in Ard.
- Direct Go imports are intentionally Go-specific.
- The compiler needs a Go package metadata loader based on `go/packages` to resolve imported Go namespaces, exported named types, exported functions, methods, and exported typed constants.
- The loader should be hidden behind an internal resolver interface so checker code does not depend directly on Go tooling details.
- Generated Go code must import directly referenced Go packages and lower namespace references to Go selectors.
- Generated Go code must also emit scalar conversions and range checks at direct Go FFI boundaries.
- Runtime validation for imported enum-like values protects Ard's closed-enum assumptions, but adds boundary code for Go returns of those types.
- Go constant aliases are supported, but they are value aliases: matching one alias makes other aliases for the same value unreachable in the same `match`.
- Existing string extern bindings remain supported, so migration can be incremental.

## Related

- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0016-defer-project-ffi-codegen.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0029-remove-javascript-targets.md`
- `compiler/go`
- `compiler/checker`
