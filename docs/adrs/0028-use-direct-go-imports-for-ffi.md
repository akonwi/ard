# 0028: Use Direct Go Imports for FFI

## Status

Proposed

## Context

ADR 0008 defines target-aware extern declarations and Go companion files as Ard's current FFI model. That model works, but it still asks users to mirror Go APIs through project-local `ffi.go` or `ffi/*.go` files even when Ard only needs to call exported Go functions, refer to exported Go types, or use exported typed constants.

That makes Go interop feel more bidirectional than necessary: users write Ard declarations, hand-write a Go companion package, then call through the generated companion package rather than the original Go package. It is still useful for adapter code, but it is too much ceremony for direct Go APIs.

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

Ard should have a one-directional Go FFI path where Ard imports Go packages and directly references exported Go types, functions, and enum-like constants. The existing companion model should remain available when an adapter layer is actually needed.

The standard library is a useful first proving ground. Many current Go standard-library bindings in `compiler/std_lib/ffi/host.go` are thin forwarding functions that exist mostly because Ard cannot name Go packages directly. Others are real adapters that translate Go conventions into Ard semantics and should remain as companion code.

The JavaScript target should not constrain this design. It is still experimental, incomplete, and not exercised enough to justify maintaining a parallel JavaScript FFI surface while redesigning Go FFI. Removing the JavaScript target should be handled as a separate prerequisite before implementing direct Go imports and migrating standard-library FFI.

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

Treat exported typed Go constants as Ard enum-like values when they have an exported named integer type. The checker should discover exported constants from imported Go packages and expose them directly under the Go namespace:

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

Implement this work after the JavaScript target has been removed. Direct Go imports are a Go-target feature, and standard-library migration should not preserve JavaScript companion modules or target-aware JavaScript extern alternatives as part of this effort.

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
- boundary adapters for `error -> Void!Str`, `(T, error) -> T!Str`, and likely `(T, bool) -> T?`;
- a way to represent direct Go packages used by embedded stdlib code with compiler-module metadata rather than a user project `go.mod`;
- clear diagnostics when a direct Go function's required parameters cannot be expressed by the Ard extern signature.

## Consequences

- Ard code can call Go packages more directly without always writing project companion FFI adapters.
- Existing `ffi.go` and `ffi/*.go` companions remain important for adapting APIs whose signatures do not map cleanly into Ard.
- Ard source keeps Ard syntax for names and types, even when the referenced symbol comes from Go.
- Go pointer interop follows Ard's `mut` reference model rather than exposing Go pointer syntax in Ard.
- Go packages become a target-specific dependency of the Go backend. Code using `use go:` is intentionally Go-specific.
- The compiler needs a Go package metadata loader based on `go/packages` to resolve imported Go namespaces, exported named types, exported functions, methods, and exported typed constants.
- The loader should be hidden behind an internal resolver interface so checker code does not depend directly on Go tooling details.
- Generated Go code must import directly referenced Go packages and lower namespace references to Go selectors.
- Runtime validation for imported enum-like values protects Ard's closed-enum assumptions, but adds boundary code for Go returns of those types.
- Go constant aliases are supported, but they are value aliases: matching one alias makes other aliases for the same value unreachable in the same `match`.
- Existing string extern bindings remain supported, so migration can be incremental.

## Related

- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0016-defer-project-ffi-codegen.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `compiler/go`
- `compiler/checker`
