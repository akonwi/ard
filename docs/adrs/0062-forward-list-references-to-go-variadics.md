# 0062: Forward List References to Go Variadics

## Status

Accepted

## Context

Ard can call imported Go variadic functions and methods with zero or repeated trailing arguments (ADR 0048):

```ard
fmt::Println()
fmt::Println("hello", 42)
```

Variadicity also survives first-class callable flow as `fn(A, ...T) R` (ADR 0058). Ard still cannot forward an existing list as the variadic tail:

```go
exec.Command("ls", args...)
```

Without forwarding, APIs such as `exec.Command`, SQL query methods, and option-based constructors need repeated call-site arguments, list reconstruction, or companion Go wrappers.

A spread operation is not an ordinary value expression. It changes call binding and passes one slice descriptor as an arbitrary number of trailing arguments. It also exposes Go aliasing: the callee receives the existing backing storage and may mutate elements or retain the slice. Ard's normal direct-Go slice policy makes that risk explicit with a reference.

The callable's Ard-facing element type is not always enough to reconstruct its exact foreign ABI. For example, these Go functions can both appear as `fn(...mut [Str])` after adaptation:

```go
func Values(values ...[]string)
func Pointers(values ...*[]string)
```

`Values` requires element projection; `Pointers` is exact. Current first-class callable type identity preserves variadicity but not that ABI distinction. A first version must not behave differently after the same callable flows through an assignment, field, parameter, or return.

## Decision

Add call-argument-only postfix spread syntax for existing variadic callable types:

```ard
use go:os/exec as exec

let args = ["-l", "/tmp"]
exec::Command("ls", (mut args)...)

let args_reference = mut args
exec::Command("ls", args_reference...)
```

The ellipsis belongs to the call argument. `value...` is not a standalone expression and cannot be bound, returned, or used outside a call.

### Call shape

For a callable equivalent to `fn(P1, P2, ...E) R`, spread form requires exactly:

```text
fixed1, fixed2, spreadReference...
```

Rules:

- the target callable must be variadic;
- exactly one spread is allowed;
- the spread must be the final source argument;
- every fixed parameter must be supplied positionally before it;
- named arguments are rejected anywhere in a spread call;
- ordinary repeated variadic elements cannot be mixed with a spread;
- nullable/default fixed parameters cannot be omitted before a spread.

These restrictions preserve one direct Go call. Go itself does not permit individual variadic elements followed by a slice spread; supporting that shape would require constructing a combined slice and would introduce hidden allocation and different aliasing.

### Explicit reference requirement

The spread operand must be an actual outer reference to one of:

- `List<E>`;
- `Slice<E>`;
- a compatible named Go slice type.

An ordinary list is rejected:

```ard
call(values...)       // rejected
call((mut values)...) // accepted
```

The reference requirement follows existing direct-Go slice boundaries. The operation reads the current descriptor from the reference; rebinding the reference slot later does not retarget a value already passed to Go.

Fixed arrays are rejected because Go only permits slices in a variadic spread.

### Whole-slice compatibility

Spread requires the dereferenced outer slice to be Go-assignable to the variadic slice type as a whole. No element-wise Ard conversion or foreign-boundary adaptation is performed.

```ard
let values: [Any] = ["hello", 42]
fmt::Println((mut values)...) // accepted: []any -> ...any

let strings = ["hello", "world"]
fmt::Println((mut strings)...) // rejected: []string is not []any
```

This preserves list invariance and avoids hidden boxing, scalar conversion, or O(n) copying. Named Go slices use Go assignability against the variadic slice type.

### Descriptor-reference element restriction

The initial implementation rejects a variadic element whose Ard shape is a reference to a descriptor, including mutable lists, slices, maps, and corresponding named Go descriptor types.

This conservatively rejects both adapted `...[]T` and exact `...*[]T` callables because current callable type identity cannot distinguish them after first-class flow. Supporting these elements later requires either:

1. preserving a spread-safety/foreign-ABI capability in callable type identity; or
2. defining an explicit allocating projection for spread.

References to non-descriptor values, such as `...*Item`, remain eligible when the outer slice is exact.

### Generic Go functions

For an imported generic Go variadic function, spread inference uses the outer container's element type as evidence for the variadic element type:

```ard
collect((mut ints)...)
```

Explicit type arguments are applied first. The instantiated whole-slice type must still pass the exact compatibility rule. A context-free empty list cannot infer the element type by itself; an explicit type argument or earlier fixed-parameter evidence is required.

### Evaluation and aliasing

A spread call evaluates:

1. the function value or method receiver once;
2. fixed arguments once in source order;
3. the spread operand once;
4. the current referenced descriptor;
5. the invocation.

Exact spread lowers directly to Go `slice...` with no element/backing-array allocation or copy. The ordinary slice-header snapshot is not a copy of its storage.

The following Go behavior is preserved:

- nil remains nil;
- a nonnil empty slice remains nonnil and empty;
- backing storage and capacity are shared;
- element writes are visible through the Ard list/reference;
- appending to or rebinding the callee's local variadic slice does not replace the Ard source descriptor;
- a `Slice<E>` keeps its capacity clipped to its visible length.

Omitting the variadic tail remains distinct from spreading an empty nonnil list.

### Ard-native variadics remain separate

This decision does not add variadic Ard declarations or closures. Declaration support needs independent decisions about syntax, the body parameter type, nil versus empty tails, mutation, generic inference, traits, and fixed/variadic callable compatibility.

The spread operation is defined against callable type shape, so a future Ard-native variadic feature may reuse it after those decisions are made.

## Lowering

The parser records spread metadata and the ellipsis span on the final call argument. The checker validates call shape, reference/container eligibility, generic inference, and whole-slice compatibility before argument information is reordered or discarded.

Checked calls and AIR carry a dedicated tail-spread marker. AIR validation requires a variadic call, one final reference-shaped slice argument, and an exact eligible element shape.

The Go backend evaluates the existing call components in source order, projects the current descriptor through the reference, and sets `ast.CallExpr.Ellipsis`. Existing result/error/Maybe adapters wrap the completed call unchanged.

All call paths follow the same rule:

- direct imported functions;
- direct imported methods;
- package and module function values;
- bound method values;
- named Go function values;
- function values stored in fields, parameters, or returns.

No runtime helper is added.

## Consequences

- Existing lists can be forwarded to common Go variadic APIs without companion wrappers.
- Mutation and retention risk remains explicit through `mut` references.
- Exact cases preserve Go's allocation and aliasing behavior.
- Spread does not become a general collection-flattening expression.
- Hidden element conversions and mixed-tail allocation are deliberately rejected.
- Descriptor-valued variadic elements remain a documented limitation until callable ABI identity is extended.
- Parser, formatter, Tree-sitter, checker call binding, Go generic inference, AIR, and every Go call path require coordinated updates.

## Related

- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0048-support-repeated-arguments-for-go-variadics.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
- `docs/adrs/0058-preserve-go-variadic-callable-types.md`
- `docs/adrs/0058-represent-list-slices-as-fixed-length-shared-views.md`
- Go specification: Passing arguments to `...` parameters
