# Idiomatic FFI: `any` type support

## Summary

Add `any` (Go's `interface{}`) as a supported idiomatic FFI type, mapping to Ard's `Dynamic`. This would let more FFI functions drop the raw `func([]*runtime.Object) *runtime.Object` signature in favor of plain Go types.

## Proposed rules

| Go type | Ard type | Unwrap (param) | Wrap (return) |
|---|---|---|---|
| `any` | `Dynamic` | `args[i].Raw()` | `runtime.MakeDynamic(result)` |

`any` works as the idiomatic way to pass/receive Ard `Dynamic` values. The Go function gets the raw underlying Go value and does its own type assertions internally.

Supported in the same positions as other idiomatic types: params, returns, `(any, error)`, etc.

## Functions that could migrate

With just `any` support (11 functions, raw count 27 → 16):

| Function | New signature | Notes |
|---|---|---|
| `IsNil` | `(any) bool` | just checks `arg == nil` |
| `GetReqPath` | `(any) string` | type-asserts `*http.Request` from Dynamic |
| `GetPathValue` | `(any, string) string` | same |
| `GetQueryParam` | `(any, string) string` | same |
| `WaitFor` | `(any)` | type-asserts `*sync.WaitGroup` |
| `JsonToDynamic` | `(string) (any, error)` | parses JSON, returns raw `any` |
| `SqlCreateConnection` | `(string) (any, error)` | returns `*sqlConnection` as opaque handle |
| `SqlClose` | `(any) error` | type-asserts `*sqlConnection` |
| `SqlBeginTx` | `(any) (any, error)` | takes connection, returns transaction |
| `SqlCommit` | `(any) error` | type-asserts `*sqlTransaction` |
| `SqlRollback` | `(any) error` | same |

### Potential `[]any` extension

Adding `[]any` (maps to `[Dynamic]`) could additionally convert:

- `ListToDynamic` → `([]any) any`
- `SqlQuery` → `(any, string, []any) ([]any, error)` (needs care with nested Object unwrapping in `sqlArgValue`)
- `SqlExecute` → `(any, string, []any) error` (same caveat)

## Functions that would remain raw regardless

These need capabilities beyond `any`:

- `DecodeString/Int/Float/Bool` — return custom error struct from embedded module lookup
- `DynamicToList`, `DynamicToMap`, `ExtractField` — return Dynamic collections + need Object for error formatting
- `MapToDynamic` — needs `map[string]any` support
- `HTTP_Send`, `HTTP_Serve` — complex multi-type with closures/structs
- `FS_ListDir` — embedded module struct construction
- `Join` — iterates Fiber struct list, extracts WaitGroups
- `JsonEncode` — generic `$T` input needs full Object for marshaling

## Relationship to opaque types

Many of the `any`-eligible functions use Dynamic as an **opaque handle** — the Ard code never inspects the value, it just passes it back to other FFI functions (e.g., SQL connection/transaction handles, HTTP request objects). If Ard gains explicit language-level support for opaque types, those would be a better fit than Dynamic, and the FFI `any` mapping might change or narrow.

## Generator implementation notes

In `ffi_generate.go`:

- `parseGoType`: recognize `*ast.Ident{Name: "any"}` or `*ast.InterfaceType` (empty interface) → `GoType{Base: "any"}`
- `generateParamUnwrap`: `any` param → `arg{i} := args[{i}].Raw()`
- `generateReturnWrap`: `any` return → `runtime.MakeDynamic(result)`
- `checkerTypeStr`: not needed for `any` unless used in `*any` or `[]any`
- For `[]any` param: iterate `.AsList()`, call `.Raw()` on each
- For `[]any` return: iterate, `MakeDynamic` each, `MakeList(checker.Dynamic, ...)`
- Import: `checker` needed for `[]any` (uses `checker.Dynamic`)
