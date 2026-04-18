# Go Transpilation Backend

This document captures the design decisions for transpiling Ard source code to Go as an alternative backend to the bytecode VM.

## Background

The current Ard compiler produces bytecode that runs on a stack-based VM. This proposal adds a Go transpilation backend that emits Go source code, which can then be compiled with `go build`.

## Transpilation Mappings

| Ard | Go | Notes |
|-----|-----|-------|
| `Int` | `int` | Direct |
| `Float` | `float64` | Direct |
| `Str` | `string` | Direct |
| `Bool` | `bool` | Direct |
| `Void` | `struct{}` or omit | |
| `List<T>` | `[]T` | Go slice, emit copy on assignment |
| `Map<K,V>` | `map[string]V` | Key coercion if K ≠ string |
| `Struct` | Go `struct` | Direct |
| `Enum` | `struct { tag int }` + const tags | Simple discriminant pattern |
| `Union` | `interface{}` + type switch | |
| `Result<T,E>` | `ard/go.Result[T,E]` | Canonical helper-backed Result value |
| `Maybe<T>` | `ard/go.Maybe[T]` struct | Custom struct preserves semantics |
| `Dynamic` | `any` + JSON helpers | |
| `extern fn` | Registry stub | Calls `ardgo.CallExtern()` |

### Copy Semantics

Ard has copy semantics for assignments. Go slices and maps share on assignment. The transpiler must emit explicit copy calls:

```ard
let a = [1, 2, 3]
let b = a  // b is a copy
```

```go
a := []int{1, 2, 3}
b := ardgo.CopySlice(a)  // explicit copy
```

### Name Rules

| Ard | Go | Rationale |
|-----|-----| -----|
| `fn my_func()` | `func MyFunc()` | Go convention: exported = PascalCase |
| `fn internal_func()` (no `pub`) | `func internalFunc()` | Unexported = camelCase |
| `utils::helper()` | `utils.Helper()` | Package-qualified call |
| `struct MyStruct` | `type MyStruct struct` | Direct |
| `enum Color` | `type Color struct { tag int }` + constants | |

Private by default. Ard uses `pub` keyword for exports — transpiler emits exported Go names only for `pub` declarations.

## CLI and Configuration

### Command Line

```bash
ard build                      # use target from ard.toml (default: bytecode)
ard build --target bytecode    # explicit bytecode
ard build --target go          # Go transpilation
ard run main.ard               # runs via bytecode VM
ard run --target go main.ard   # transpile + go run
```

### `ard.toml` Configuration

```toml
name = "myproject"
version = "0.1.0"
target = "bytecode"  # "bytecode" (default) or "go"
```

The `target` field specifies the default backend. The `--target` flag overrides it.

### Output

- **Bytecode**: Single executable with embedded program
- **Go backend**: `generated/` directory with `go.mod` and Go packages

## Module Mapping

Each `.ard` file becomes one Go package.

### Directory Structure

**Ard project:**
```
myproject/
  ard.toml                  # name = "myproject"
  main.ard
  utils.ard
  helpers/
    math.ard
```

**Transpiled Go:**
```
myproject/
  generated/
    go.mod                  # module github.com/user/myproject (from ard.toml name)
    main/
      main.go               # package main
    utils/
      utils.go              # package utils
    helpers/
      math/
        math.go             # package math
```

The Go module path is derived from `ard.toml`'s `name` field.

### Import Translation

```ard
// main.ard
use ./utils
use ./helpers/math

fn main() {
  let a = utils::double(5)
  let b = math::add(3, 4)
}
```

```go
// generated/main/main.go
package main

import (
    "github.com/user/myproject/generated/utils"
    math "github.com/user/myproject/generated/helpers/math"
)

func main() {
    a := utils.Double(5)
    b := math.Add(3, 4)
}
```

## Standard Library

The standard library (`std_lib/*.ard`) is pre-transpiled to Go packages shipped with the compiler.

```
compiler/go/
  maybe.go
  copy.go
  dynamic.go
  enum.go
  extern.go
  stdlib/
    json/
      json.go              # pre-transpiled from std_lib/json.ard
    http/
      http.go              # pre-transpiled from std_lib/http.ard
    async/
      async.go             # pre-transpiled from std_lib/async.ard
    ...
```

User code imports these as normal Go packages.

## Extern Functions

Extern functions (`extern fn`) generate stub functions that call a registry at runtime.

```ard
// user code
extern fn my_extension(x: Int) Str = "MyExtension"
```

```go
// generated stub
func myExtension(x int) string {
    result, err := ardgo.CallExtern("MyExtension", x)
    if err != nil {
        panic(err)
    }
    return result.(string)
}
```

The registry maps binding names to Go implementations, mirroring the current FFI model. Implementations live in `ard/go/stdlib/` or are registered by user code.

### Async Semantics

The current VM uses `sync.WaitGroup` with child goroutines. The Go backend preserves this pattern:

```ard
let fiber = async::start(fn() { do_work() })
fiber.join()
```

```go
fiber := async.Start(func() { doWork() })
fiber.Join()
```

The `async` stdlib module provides this as pre-transpiled Go code.

## Helper Package: `ard/go`

Core helper types and functions for transpiled code:

```
compiler/go/
  maybe.go          # Maybe[T] struct
  copy.go           # Copy functions for slices/maps
  dynamic.go        # Dynamic type + JSON helpers
  enum.go           # Enum tag construction
  extern.go         # Extern function registry
  result.go         # Result[T,E] helper
```

### `maybe.go`

```go
package ardgo

type Maybe[T any] struct {
    value T
    none  bool
}

func Some[T any](v T) Maybe[T] { return Maybe[T]{value: v} }
func None[T any]() Maybe[T] { return Maybe[T]{none: true} }
func (m Maybe[T]) IsNone() bool { return m.none }
func (m Maybe[T]) IsSome() bool { return !m.none }
func (m Maybe[T]) Expect(msg string) T {
    if m.none {
        panic(msg)
    }
    return m.value
}
func (m Maybe[T]) Or(default_ T) T {
    if m.none {
        return default_
    }
    return m.value
}
```

### `result.go`

```go
package ardgo

type Result[T, E any] struct {
    value T
    err   E
    ok    bool
}

func Ok[T, E any](value T) Result[T, E] {
    return Result[T, E]{value: value, ok: true}
}

func Err[T, E any](err E) Result[T, E] {
    return Result[T, E]{err: err}
}

func (r Result[T, E]) IsOk() bool { return r.ok }
func (r Result[T, E]) IsErr() bool { return !r.ok }
func (r Result[T, E]) Or(fallback T) T {
    if r.ok {
        return r.value
    }
    return fallback
}
func (r Result[T, E]) Expect(message string) T {
    if !r.ok {
        panic(message)
    }
    return r.value
}
func (r Result[T, E]) UnwrapOk() T { return r.value }
func (r Result[T, E]) UnwrapErr() E { return r.err }
```

### `copy.go`

```go
package ardgo

func CopySlice[T any](s []T) []T {
    copied := make([]T, len(s))
    copy(copied, s)
    return copied
}

func CopyMap[K comparable, V any](m map[K]V) map[K]V {
    copied := make(map[K]V, len(m))
    for k, v := range m {
        copied[k] = v
    }
    return copied
}
```

### `enum.go`

```go
package ardgo

type Enum struct {
    Tag   int
    Value any
}

func MakeEnum(tag int, value any) Enum {
    return Enum{Tag: tag, Value: value}
}
```

### `extern.go`

```go
package ardgo

var externRegistry = make(map[string]func(...any) (any, error))

func RegisterExtern(name string, fn func(...any) (any, error)) {
    externRegistry[name] = fn
}

func CallExtern(name string, args ...any) (any, error) {
    fn, ok := externRegistry[name]
    if !ok {
        return nil, fmt.Errorf("extern function not found: %s", name)
    }
    return fn(args...)
}
```

## Implementation Plan

### Phase 1: Core Helpers

Create `compiler/go/` package with:
- `maybe.go` — Maybe[T] implementation
- `copy.go` — Copy functions
- `enum.go` — Enum construction helpers
- `extern.go` — Extern registry

### Phase 2: Minimal Transpiler

Create `compiler/transpile/` package that handles:
- Function definitions with parameters and return types
- Primitive types (Int, Float, Str, Bool)
- Basic expressions (arithmetic, comparisons)
- Variable declarations and assignments
- Control flow (if/else, while, for)

### Phase 3: Type Declarations

Add support for:
- Struct definitions
- Enum definitions
- Union types

### Phase 4: Collections and Pattern Matching

Add support for:
- List and Map literals and operations
- Pattern matching on enums, unions, integers
- Result and Maybe types

### Phase 5: CLI Integration

Update `compiler/main.go` to:
- Read `target` from `ard.toml`
- Accept `--target` flag
- Emit Go output to `generated/` directory
- Invoke `go build` forGo target

### Phase 6: Stdlib Transpilation

Run transpiler on all `std_lib/*.ard` files and output to `compiler/go/stdlib/`.

### Phase 7: Parity Testing

Create test infrastructure to run the same tests through both backends and compare outputs.

## Decisions Summary

| Aspect | Decision |
|--------|----------|
| Result | `ard/go.Result[T,E]` |
| Maybe | `ard/go.Maybe[T]` struct |
| Enum | `struct { tag int }` + const |
| Union | `interface{}` + type switch |
| Dynamic | `any` + JSON helpers |
| Module mapping | One .ard file → One Go package |
| CLI override | `build`/`run` support `--target <value>`; `test` uses config target |
| Config | `target` field in `ard.toml` |
| Stdlib | Pre-transpiled Go packages |
| Externs | Registry + stub functions |
| Copy semantics | Explicit `ardgo.Copy*()` calls |

## Open Questions

1. **Go module path derivation** — How to map `ard.toml` name to Go module path?
   - Option A: Use name directly (e.g., `name = "myproject"` → `module myproject`)
   - Option B: Require full path in config (e.g., `module = "github.com/user/myproject"`)

2. **Circular dependencies** — Go requires all imports to exist. How to handle circular module dependencies in Ard?

3. **Dependency ordering** — Transpiler needs to emit packages in dependency order. Might need a dependency graph analysis pass.

## Go Backend Test Mode Sketch

This section sketches how Ard's existing test model should map onto the Go backend. This is not implemented yet, but it should become the path for validating transpiled code once the backend is further along.

### Backend selection

`ard test` should **not** take a `--target` flag.

Instead:
- `ard build` / `ard run` may continue to support `--target`
- `ard test` should use the backend selected in `ard.toml`
- if `target` is omitted, `ard test` should default to bytecode just like the rest of the toolchain

### Goals

- Preserve Ard's current test semantics: `test fn`, `Void!Str`, pass/fail/panic
- Preserve the visibility split between co-located tests and `/test` integration tests
- Reuse Go's `go test` runner when the configured backend is Go
- Provide a future validation harness for transpiled code parity

### Lowering model

An Ard test should remain an Ard-shaped function in the generated code.

For example:

```ard
test fn adds() Void!Str {
  try testing::assert(1 + 1 == 2, "bad math")
  testing::pass()
}
```

should lower conceptually to:

```go
func ardTestAdds() ardgo.Result[struct{}, string] {
    // transpiled Ard body
}

func TestAdds(t *testing.T) {
    res := ardTestAdds()
    if res.IsErr() {
        t.Fatal(res.UnwrapErr())
    }
}
```

This keeps the source-of-truth semantics in Ard itself:
- `Result::ok(())` => pass
- `Result::err(msg)` => fail
- panic => panic/failing Go test

`testing::assert`, `testing::fail`, and `testing::pass` should stay ordinary Ard stdlib functions. They should **not** be rewritten directly into `testing.T` calls.

### Co-located tests

Co-located tests should be emitted into the **same Go package** as the Ard module they live beside.

That preserves Ard's intended unit/internal-test behavior:
- same-module visibility
- access to private/internal declarations already allowed by the checker

Implementation shape:
- emit the regular transpiled module file as usual
- emit a sibling `_test.go` wrapper file in the same Go package
- include transpiled test functions only in test mode

### `/test` integration tests

Files under `/test` should be treated as ordinary external Ard modules and transpiled as their own Go packages in test mode.

That preserves Ard's intended integration/public-API behavior:
- no special private access
- imports must go through the public module surface
- checker remains the authority on visibility

Each such package should also get `_test.go` wrappers that call the transpiled Ard test functions.

### Test-mode output shape

The Go backend should have a separate **test mode** rather than reusing normal production emission unchanged.

Test mode should:
- include `test fn` declarations instead of skipping them
- emit `_test.go` wrappers
- generate a temporary or dedicated test-only Go project tree
- invoke `go test` instead of `go build` / `go run`

Prefer keeping this output separate from normal `generated/` production builds so that test-only artifacts do not leak into ordinary transpilation output.

### Runner behavior

When `target = "go"`, `ard test` should conceptually do:

1. discover Ard tests exactly as today (`test fn` is source of truth)
2. transpile the project in test mode
3. run `go test` over the generated test project
4. report pass/fail/panic through the existing Ard CLI UX

Existing Ard test CLI options can map naturally:
- `--filter` -> `go test -run`
- `--fail-fast` -> `go test -failfast`

### Why this matters for backend validation

Once this exists, the Ard test suite itself becomes a backend validation tool.

That gives us a natural path to:
- run Ard tests through the transpiled Go backend
- use real project tests as validation for new lowering work
- later compare bytecode and Go backend results for parity on the same test corpus

In other words, Go-backed `ard test` is not just a product feature; it can also become the main semantic regression harness for the transpiler.
