# 0077: Add Tuple Types for Go Multi-Result Interop

## Status

Accepted. Extends ADR 0038.

## Context

A Go function may return any number of results. Ard adapted only three
shapes: a single result, `(T, error)` as `T!Error`, and `(T, bool)` as `T?`
(ADR 0038). Every other multi-result signature was rejected, so the function
could not be called from Ard at all (#278).

A scan of the Go standard library found 5,222 exported functions and methods,
of which 1,131 return more than one result. The existing adaptations cover
953 of those. The other 178 were unusable:

| Shape | Count | Examples |
| --- | --- | --- |
| two or more results, no trailing `error` or `bool` | 108 | `context.WithCancel`, `io.Pipe`, `filepath.Split`, `math.Modf`, `time.Time.Date` |
| three or more results ending in `error` | 59 | `os.Pipe`, `bufio.Reader.ReadRune`, `net.SplitHostPort` |
| three or more results ending in `bool` | 11 | `runtime.Caller`, `reflect.Select`, `strings.Cut` |

The same shapes appear in callback parameters and interface methods, so
interfaces such as `io.RuneReader` could not be implemented from Ard either.

Ard had no type that holds several unnamed values. Three alternatives were
considered:

- **Synthesize a struct per signature**, with fields named after the Go
  results. Go result names are optional and are not part of a function's
  type, so two functions with the same signature would return different Ard
  types, and unnamed results would need invented field names. It also gives
  Ard code no way to write the type of a multi-result callback.
- **Adapt each remaining shape to an existing type**, for example a trailing
  `bool` after several values as a Maybe of the rest. A trailing `bool` is
  not reliably an "ok" flag: `reflect.Select` returns `(chosen int, recv
  Value, recvOK bool)`, where `chosen` is valid even when `recvOK` is false.
  This also leaves the 108 signatures with no trailing flag unsupported.
- **Add tuples to the language.** One rule covers every remaining shape, the
  types are structural so equal signatures give equal types, and Ard code can
  name the type.

## Decision

### Tuple types

Ard has tuple types, written as two or more comma-separated types in
parentheses:

```ard
fn min_max(values: [Int]) (Int, Int) {
  (values.at(0), values.at(1))
}
```

- A tuple has at least two elements. `()` remains the Void literal, and
  `(T)` and `(value)` remain grouping. There is no one-element tuple.
- A trailing comma is allowed after the last element. `(T,)` and `(value,)`
  are errors, not one-element tuples.
- Tuple types are structural. Two tuple types are the same type when they
  have the same number of elements and the same element types in order.
- A tuple literal is typed from its elements, or from the expected tuple type
  when there is one, so `let pair: (Int, [Str]) = (1, [])` checks.
- Tuples can be used wherever a type can: bindings, parameters, results,
  struct fields, collection elements, generic arguments, and under `?` and
  `!`.

### Destructuring is the only way to read a tuple

```ard
let (low, high) = min_max(values)
mut (count, _) = totals()
```

- The names in parentheses must match the tuple's element count. `_` discards
  an element and binds nothing.
- `let` makes every binding immutable and `mut` makes every binding mutable.
- The value is evaluated once.
- The value must be a tuple. A `(A, B)?`, `(A, B)!E`, or `&(A, B)` must be
  unwrapped or read first, and the diagnostic says how.
- Names in one destructuring must be distinct.
- Destructuring takes no type annotation. Annotate the tuple by binding it to
  one name first.
- Destructuring is a statement in a function body. It is not allowed at the
  top level of a module, where `let` and `mut` declare a single global
  (ADR 0075).

Tuples are deliberately small in this first version. They have no member
access (`pair.0`), no methods, no `==`, no `match` patterns, no string
interpolation, and are not iterable. They cannot be map keys and do not
satisfy a `comparable` constraint. Each of these can be added later without
changing the meaning of code that compiles today. A value that needs named
parts, equality, or methods should be a struct.

### Go results map to tuples

The existing adaptations are unchanged and apply first. Tuples cover what is
left:

| Go results | Ard return type |
| --- | --- |
| `T` | `T` |
| `(T, error)` | `T!Error` |
| `(T, bool)` | `T?` |
| `(A, B, ..., error)`, three or more results | `(A, B, ...)!Error` |
| any other two or more results | `(A, B, ...)` |

- A trailing `error` is always the failure channel, because Go reserves it
  for that. When it is non-nil the other results are not read.
- A trailing `bool` is the Maybe flag only in the two-result `(T, bool)`
  shape. After two or more values it stays a tuple element, so
  `runtime.Caller` returns `(Uintptr, Str, Int, Bool)`.
- Each element is converted by the same rules as a single result. If an
  element type is unsupported, the function is rejected and the diagnostic
  names the result position.

The mapping applies everywhere a Go signature is read: package functions,
methods, generic functions, function-typed values and fields, callback
parameters, and interface methods.

### Tuple returns use Go multiple results

ADR 0038 gave Result and Maybe returns an idiomatic Go ABI. Tuples follow the
same contract:

| Ard return type | Go results |
| --- | --- |
| `(A, B)` | `(A, B)` |
| `(A, B)!E` | `(A, B, error)` |
| `(A, B)?` | `(struct{ V0 A; V1 B }, bool)` |

- An Ard function, method, or closure that returns a tuple is a Go function
  with one result per element. An Ard closure can therefore be passed
  directly as a Go callback with multiple results, and an Ard type can
  implement a Go interface whose methods have multiple results.
- A tuple *value* (in a variable, a field, a collection, or under `?`) is a
  Go anonymous struct with fields `V0`, `V1`, and so on. Anonymous structs
  with the same fields are identical Go types, so no declaration is emitted
  and the same tuple type is interchangeable across packages.
- The backend packs Go results into the struct at a call that produces a
  tuple value, and spreads the struct into results at a `return`.
- As in ADR 0038, the layout follows the *declared* type. A function whose
  declared return type is a type parameter returns one value even when the
  type argument is a tuple, and the backend adapts function values where a
  tuple-returning function meets a `fn() $T` parameter.

### Pipeline

- The parser adds `TupleType`, `TupleLiteral`, and `TupleDestructure` nodes.
- The checker adds the `Tuple` type and the foreign result shapes
  `ForeignResultTuple` and `ForeignResultTupleError`.
- AIR adds `TypeTuple` with interned element types, and the expressions
  `ExprMakeTuple` and `ExprTupleElem`. Destructuring lowers to one temporary
  and one `ExprTupleElem` per bound name, so backends need no destructuring
  construct.
- The formatter, language server, and Go backend handle the new nodes.

## Consequences

- The 178 standard-library functions and methods listed above can be called
  from Ard, as can third-party functions with the same shapes. No Go result
  shape is rejected for its arity alone.
- Interfaces with multi-result methods, such as `io.RuneReader`, can be
  implemented from Ard, and Go APIs that take multi-result callbacks can be
  given Ard closures.
- Signatures that were already supported keep their Ard types, so existing
  code is unaffected. `(T, bool)` stays `T?` even where the bool is not an
  "ok" flag, as before.
- Reading a tuple always names its elements at the use site, which keeps
  code that consumes Go results readable without positional access.
- Code that needs to compare, print, or key on a tuple must destructure it
  first. This is the cost of the small first version.
- A tuple that reaches Go as a value, for example through `Any` or a generic
  type argument, is the anonymous struct, not multiple values. Only return
  position spreads.
- The tree-sitter grammar needs the tuple type, literal, and destructuring
  forms for editor highlighting.

## Related

- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/adrs/0053-add-a-builtin-error-contract-for-go-interop.md`
- `docs/adrs/0075-allow-only-declarations-at-module-top-level.md`
- `compiler/parse`
- `compiler/checker`
- `compiler/air`
- `compiler/go`
