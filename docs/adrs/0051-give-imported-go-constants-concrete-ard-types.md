# 0051: Give Imported Go Constants Concrete Ard Types

## Status

Accepted

## Context

Go supports untyped constants. An untyped constant is a compile-time value that can adopt a compatible concrete type from context, subject to representability checks. This lets declarations such as `const None = 0` flow implicitly into `int`, `uint16`, or named scalar types.

Ard currently maps imported untyped Go constants to concrete Ard types:

- untyped boolean to `Bool`;
- untyped string to `Str`;
- untyped integer or rune to `Int`;
- untyped float to `Float64`.

As a result, an imported untyped integer constant does not implicitly become a foreign named scalar:

```ard
use go:io
use go:time

let offset: time::Duration = io::SeekStart
```

The assignment is rejected because `io::SeekStart` is an Ard `Int`, while `time::Duration` is a distinct foreign named scalar. The same distinction applies to function arguments and mixed arithmetic.

Matching Go exactly would require Ard to preserve untyped-constant provenance and exact constant values through Go package metadata and expression checking. Contextual typing would then need to distinguish compile-time constants from runtime values, select target types, check representability, and carry the selected type through AIR and target lowering.

Ard already has explicit scalar conversions through `T::from(value)`. The added type-system complexity of Go-style untyped constants is not justified by the ergonomics it provides.

## Decision

Ard will not model Go's untyped constants as context-sensitive Ard values.

When imported, a Go constant receives a concrete Ard type immediately. Untyped constants use the mappings above; Go constants declared with a named or otherwise concrete supported type retain that mapped type.

An imported constant follows the same compatibility rules as any other Ard expression of its concrete type. It does not implicitly adopt a type from:

- an annotated binding;
- an assignment target;
- a function parameter;
- a return type;
- the other operand of an arithmetic or comparison expression.

Conversion to a different scalar type must be explicit:

```ard
use go:io
use go:time

let offset = time::Duration::from(io::SeekStart)
time::Sleep(offset)
```

For a foreign named scalar constant block that mixes typed and untyped declarations, callers similarly convert the untyped members:

```ard
let mask = ui::AttributeMask::from(ui::AttrNone)
let combined = mask + ui::AttrBold
```

Source-level Ard numeric literals remain contextually typed where Ard already supports literal adoption. This is an Ard literal rule, not a general untyped-constant system:

```ard
let offset: time::Duration = 0
```

Runtime `Int` and `Float64` values continue to require the same explicit conversions as imported constants.

## Consequences

- Ard's type system does not need a representation for untyped constants or arbitrary-precision imported constant values.
- Imported values have stable concrete types independent of their use site.
- Checker behavior stays consistent across bindings, calls, returns, and operators.
- Some Go APIs are less concise from Ard when they expose untyped constants intended for named scalar contexts.
- `T::from(...)` is the standard migration and interop mechanism for those APIs.
- Ard intentionally differs from Go even though generated Go could accept some of these assignments implicitly.
- Documentation and diagnostics should recommend explicit conversion when a concrete imported constant is used where another scalar type is expected.

## Non-goals

This decision does not change:

- contextual adoption of source-level Ard numeric literals;
- import of typed Go constants;
- explicit scalar conversion semantics;
- Go's handling of constants inside Go FFI or shim packages.

## Related

- Issue [#301](https://github.com/akonwi/ard/issues/301)
- `compiler/checker/go_import.go` (`constTypeFromGo`)
- `compiler/checker/checker.go` (`isUntypedNumLiteral`, `checkScalarOperands`)
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0036-define-any-casting-policy.md`
