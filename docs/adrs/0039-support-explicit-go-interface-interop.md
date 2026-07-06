# 0039: Support Explicit Go Interface Interop

## Status

Accepted

## Context

Ard now imports Go packages directly through `use go:` and can call functions, use constants, work with named foreign types, call methods, access struct fields, construct Go structs, pass callbacks, interoperate with maps, and use `Any` for Go `any` values. ADR 0038 also changed Go lowering so `Result[T, Str]` and `Maybe[T]` use idiomatic Go ABI shapes in function and method return position:

- `T!Str` lowers to `(T, error)`;
- `Void!Str` lowers to `error`;
- `T?` lowers to `(T, bool)`;
- `Void?` lowers to `bool`.

This makes Ard methods capable of matching common Go interface methods such as `io.Writer.Write([]byte) (int, error)` without special result adapters.

The remaining gap is Go interface interop. Today non-empty Go interface types are rejected by direct Go import resolution. This prevents Ard from using many idiomatic Go APIs whose parameters, results, fields, or methods mention interfaces such as `io.Reader`, `io.Writer`, `fmt.Stringer`, `http.ResponseWriter`, or package-defined callback interfaces.

Ard also has its own trait system. Trait implementation is explicit:

```ard
impl Trait for Type {
  fn method(...) ...
}
```

That explicitness is part of Ard's source semantics. Go, by contrast, uses structural interface satisfaction. The Go backend may naturally emit methods whose generated Go type structurally satisfies a Go interface, but Ard source should not treat accidental method shape as intentional interface implementation.

## Decision

Support named Go interface types as foreign Go types and support explicit Ard implementation of those interfaces.

### Go interfaces are foreign types, not Ard traits

A named Go interface imported through `use go:` is represented as a foreign Go interface type. It is not converted into an Ard trait and does not participate in ordinary Ard trait lookup.

```ard
use go:io

fn consume(writer: io::Writer) {
  // writer is a foreign Go interface value
}
```

A foreign Go interface value can be stored, passed, returned, and used at direct-Go boundaries. It preserves Go interface semantics, including nil interface state. Go nil does not become Ard `Maybe`; wrappers that want absence semantics must adapt it explicitly, and `ard/unsafe::is_nil` remains the tool for inspecting foreign nil state.

Pointer-to-interface types remain unsupported. Go interface values are already reference-like descriptors, and accepting `*interface` shapes should wait for a concrete use case.

### Interface methods

The checker should load the exported method set of a named Go interface. A method is available only when its parameters and results are representable by Ard's existing direct-Go boundary rules.

Method calls on foreign Go interface values use the same surface syntax as other method calls:

```ard
use go:io

fn write_one(mut writer: io::Writer) Int!Str {
  writer.Write([1])
}
```

The method's Ard-facing signature is derived from the Go signature using the same boundary rules as direct Go functions and methods. For example, `Write([]byte) (int, error)` is exposed as returning `Int!Str`.

If an interface contains unsupported methods, diagnostics should name the unsupported method and the unsupported Go shape. A future implementation may choose whether an interface type is rejected entirely when any method is unsupported, or whether the type is accepted while unsupported methods are unavailable. The preferred user experience is actionable diagnostics at the use site.

### Go-owned concrete values should satisfy Go interfaces by Go assignability

When both the source value and target interface are Go-owned foreign types, Ard should follow Go assignability.

For example, if a Go package returns `*bytes.Buffer`, and a direct-Go API expects `io.Reader`, the checker should accept the call when Go metadata says the concrete type is assignable to the interface.

This does not require an Ard `impl`, because both sides are Go-owned and the semantics are Go's own structural interface semantics.

### Ard-owned values require explicit implementation

An Ard-owned type is assignable to a foreign Go interface only when Ard source contains an explicit implementation block for that interface:

```ard
use go:io

struct Sink {
  written: Int,
}

impl io::Writer for Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    bytes.size()
  }
}
```

The explicit `impl` is required even if the type has an inherent method whose generated Go method would structurally satisfy the Go interface:

```ard
impl Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    bytes.size()
  }
}
```

The generated Go for that inherent method may still accidentally satisfy a Go interface because Go is structural, but Ard's checker must not treat the Ard type as assignable to the foreign interface without the explicit `impl io::Writer for Sink`.

This preserves Ard's explicit implementation model while still allowing generated Go to remain idiomatic.

### Explicit foreign-interface impl validation

For an `impl ForeignInterface for ArdType` block, the checker validates against the Go interface method set:

- the impl target must resolve to a named foreign Go interface type;
- the receiver type must be Ard-owned;
- every required Go interface method must be implemented;
- extra methods follow the same policy as Ard trait impls;
- Go method names map to Ard method names using the normal Ard-to-Go identifier conversion, reversed for lookup (`Write` ↔ `write`, `ReadFrom` ↔ `read_from`);
- parameters must be compatible under direct-Go boundary rules;
- return shapes must be compatible with ADR 0038 return-position ABI rules;
- a mutating Ard method requires an addressable mutable value when upcast to the interface.

Common result/maybe mappings are valid in interface methods:

- Go `(T, error)` ↔ Ard `T!Str`;
- Go `error` ↔ Ard `Void!Str`;
- Go `(T, bool)` ↔ Ard `T?`;
- Go `bool` for an optional unit result ↔ Ard `Void?`.

### Lowering explicit foreign-interface impls

The Go backend emits real Go methods with the interface's Go method names and signatures.

For the `io.Writer` example:

```ard
impl io::Writer for Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    bytes.size()
  }
}
```

The backend emits a pointer receiver method because `self` is mutable:

```go
func (s *Sink) Write(bytes []byte) (int, error) {
  // lowered Ard body
}
```

Because ADR 0038 makes `Int!Str` lower to `(int, error)` in method return position, no special result wrapper is needed for this common case.

If an Ard type has multiple methods that would lower to the same Go receiver method name/signature, the checker or backend must reject the duplicate rather than emitting invalid Go. Inherent method naming and foreign-interface impl method naming share the same generated Go method namespace.

### Interface assignment and call compatibility

An Ard-owned value may be assigned to or passed as a foreign Go interface only if the explicit foreign-interface impl exists.

If the required impl methods are non-mutating, the value may be passed according to ordinary value rules. If any required impl method is mutating and therefore lowers with a pointer receiver, the value must be mutable and addressable at the upcast/call site.

The initial implementation supports pointer-required upcasts for mutable local bindings and mutable Ard struct fields. Broader addressable foreign places can be added as the checker and AIR grow a more general addressable-place representation for foreign-interface upcasts.

```ard
mut sink = Sink { written: 0 }
needs_writer(sink) // valid when Sink explicitly impls io::Writer with mutable write
```

The backend may pass `&sink` or otherwise use the pointer receiver representation needed by Go.

### Relationship to Ard traits

Foreign Go interfaces do not replace Ard traits. Ard traits remain the core language abstraction for shared behavior in Ard source, and their implementation remains explicit.

A future design may add explicit bridging between Ard traits and specific Go interfaces, but this ADR does not make arbitrary Ard traits equivalent to arbitrary Go interfaces. The only interface satisfaction described here is:

- Go-owned value to Go-owned interface by Go assignability;
- Ard-owned value to Go-owned interface by explicit `impl go::Interface for ArdType`.

## Consequences

- Ard can use many more idiomatic Go APIs that depend on interfaces.
- Ard source keeps explicit implementation semantics instead of adopting accidental structural satisfaction.
- Generated Go remains idiomatic and may naturally satisfy Go interfaces.
- ADR 0038's idiomatic return ABI becomes important for interface method compatibility.
- Go interface nil behavior remains a foreign value-state concern governed by ADR 0037.
- Pointer-to-interface, broader foreign addressable upcasts, and adapter generation for unsupported method shapes remain deferred.
- Diagnostics must clearly distinguish unsupported Go interface shapes from missing explicit Ard impls.

## Related

- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/adrs/0037-define-unsafe-nil-interop-policy.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
