# 0050: Expose Embedded Go Fields Without Field Promotion

## Status

Accepted

## Context

Go permits a struct field to be embedded by declaring the field's type without a separate field name:

```go
type Base struct {
    Name string
}

type Outer struct {
    Base
}
```

The embedded field is still real nested storage named `Base`. Go permits both explicit access (`outer.Base.Name`) and promoted access (`outer.Name`). Go also promotes methods from embedded fields into the outer type's method set when lookup is unambiguous.

Direct Go FFI already imports Go method sets, so unambiguous promoted methods work in Ard. Direct Go field discovery currently skips embedded fields entirely. As a result, Ard cannot access `outer.Base` or initialize `Base` in a keyed `Outer` literal.

Supporting promoted fields would require Ard to reproduce Go's selector-depth, shadowing, ambiguity, embedded-pointer, and nil traversal rules. Ard's own struct model uses explicit named composition and does not otherwise have field embedding or promotion.

## Decision

Direct Go FFI will expose exported embedded fields as directly declared fields of imported Go structs, but Ard will not support promoted field lookup.

Given the Go declarations above, Ard supports explicit nested access:

```ard
let base = outer.Base
let name = outer.Base.Name
```

Ard does not support omitting the embedded selector:

```ard
let name = outer.Name // undefined
```

An exported embedded field can be supplied by its declared Go field name in a keyed literal:

```ard
let outer = ffi::Outer{
  Base: ffi::Base{Name: "Ard"},
}
```

A promoted child field cannot be used as a keyed literal field:

```ard
let outer = ffi::Outer{
  Name: "Ard", // unknown field
}
```

Field assignment and mutable-reference behavior follow the same rules as other directly exposed Go fields. Explicit paths such as `outer.Base.Name` are writable when the receiver is mutable and each foreign type is otherwise representable.

### Embedded pointers

An exported embedded pointer field is exposed when its pointer shape is representable by Direct Go FFI. Ard accesses it through the explicit field selector. Go nil behavior is preserved; Ard does not insert a safe-navigation or promotion layer.

### Methods

Existing Go method-set behavior remains unchanged. Unambiguous promoted methods remain callable because they are members of the imported Go method set:

```ard
outer.Hello()
```

Ard does not introduce native struct embedding, field promotion, or method promotion syntax. This decision applies only to faithfully exposing directly declared fields and existing method sets of imported Go types.

## Consequences

- Ard can access and initialize the actual nested storage of imported structs that use Go embedding.
- Imported Go struct layouts are more faithfully represented without adding Go's promoted-field lookup model to Ard.
- Users must spell explicit field paths, making ownership and nil traversal visible.
- Promoted fields continue to produce ordinary undefined/unknown-field diagnostics.
- Keyed literal behavior matches Go: the embedded field name is valid, while promoted child field names are not.
- Unambiguous promoted methods continue to work through Go method sets, creating an intentional distinction between method-set interop and field selector promotion.
- Go APIs that rely exclusively on promoted field shorthand remain usable through explicit nested selectors or small Go adapters.

## Related

- `docs/adrs/0028-use-direct-go-imports.md`
- `docs/adrs/0030-use-direct-go-struct-values-and-fields.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `compiler/checker/go_import.go`
- `compiler/checker/checker.go`
- GitHub issue #249
