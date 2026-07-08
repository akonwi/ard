# 0046: Support Function-Scoped Defer

## Status

Accepted

## Context

Ard programs that interoperate directly with Go need a lightweight cleanup mechanism for resources whose lifecycle is tied to a function call:

```ard
let file = try os::Open(path)
defer file.Close()

let rows = try db.Query(query)
defer rows.Close()
```

Go already has `defer`, and Ard's Go backend can use it as the target primitive. The language still needs Ard-owned semantics rather than importing every Go rule verbatim.

Two design axes matter:

1. **Scope.** Go's `defer` runs at function exit. Zig's `defer` runs at lexical scope exit.
2. **Evaluation time.** Go evaluates the deferred call's callee and arguments immediately, then runs the call later. Zig treats the deferred statement as later work.

Ard blocks are already lexical scopes and block expressions can produce values, so block-scoped defer was considered. It is semantically attractive, but it is not a direct fit for the current Go backend. Emitting a Go block is insufficient because Go `defer` remains function-scoped, and lowering every Ard block to an immediately invoked function expression changes outward control flow: `try` would return from the IIFE rather than the enclosing Ard function, and `break` could not exit an outer loop. Preserving those semantics would require a control-signal lowering transform.

The simpler feature is function-scoped defer with late evaluation. It matches Go's resource-lifetime model while avoiding Go's immediate-argument-evaluation surprise. The divergence from Go is narrower than it first appears: immutable `let` bindings and per-iteration loop bindings observe the same values under both models. The observable difference is primarily a deferred expression that captures a `mut` binding that is reassigned before function exit. Ard chooses the closure-capture behavior because it is uniform with block-form defer; snapshot behavior remains explicit through a `let` binding.

## Decision

Add `defer` as a keyword and statement form. `defer` schedules work to run when the current function or method exits.

Supported forms:

```ard
defer call_expression

defer {
  statements
  final_expression
}
```

`defer call_expression` is shorthand for scheduling a zero-argument deferred closure containing that call. The call is not evaluated when execution reaches the `defer`; it is evaluated when the deferred closure runs.

```ard
mut label = "open"
defer log(label)
label = "closed"
// logs "closed" when the function exits
```

If snapshot behavior is desired, bind the snapshot explicitly:

```ard
let saved = label
defer log(saved)
```

### Function-scoped lifetime

A deferred operation is registered only if execution reaches the `defer` statement. Registered defers run in last-in-first-out order when the enclosing function exits.

For this ADR, "function" means a named function, method, script entry function, or closure literal. A closure literal is its own defer boundary: a `defer` inside `fn() { ... }` runs when that closure call exits, not when the lexically enclosing function exits. A deferred block is also lowered as a closure body, so a nested `defer` inside deferred work runs when that deferred closure exits.

The function-exit paths are:

- normal fallthrough / final expression;
- `try` propagation;
- panic unwinding, following the Go backend's panic behavior.

`break` exits loops, not functions, so it does not run function-scoped defers unless it is followed by a function exit.

Defers inside loops are still function-scoped:

```ard
for item in items {
  defer cleanup(item)
}
// all registered cleanups run at function exit, in reverse registration order
```

They do not run at the end of each loop iteration or at the end of the loop body. Per-iteration cleanup should be written explicitly or introduced later as a separate scope-exit feature.

### Evaluation and capture

Deferred work is always lowered as a zero-argument closure:

```ard
defer resource.Close()

defer {
  log("closing")
  resource.Close()
}
```

conceptually lowers to:

```go
defer func() {
    resource.Close()
}()

defer func() {
    log("closing")
    resource.Close()
}()
```

This deliberately differs from Go's `defer resource.Close()`, which evaluates the receiver and arguments immediately. Ard's rule is uniform: both call-form and block-form defer evaluate their body later, using ordinary closure capture semantics.

Because this is ordinary closure capture, a deferred expression that reads a reassigned `mut` binding sees the later binding value. If a cleanup must close the current resource before the binding is reassigned, snapshot it first:

```ard
let current = resource
defer current.Close()
resource = next_resource
```

### Results are discarded

Any value produced by deferred work is discarded. This is consistent with ordinary Ard statement position, where a call's value can already be ignored. It also includes Go cleanup methods that return `error`, which Ard adapts as `Void!Str`:

```ard
defer file.Close() // cleanup result is ignored
```

If cleanup failure matters, handle it inside a deferred block:

```ard
defer {
  match file.Close() {
    Result::err(e) => log::warn("close failed: {e}"),
    Result::ok(_) => (),
  }
}
```

A deferred cleanup failure does not replace or modify the enclosing function's result unless the program explicitly records that state outside the deferred block.

Panics are not discarded values. A panic in deferred work follows Go semantics: it propagates during function unwinding, and Go's normal panic chaining rules apply if another panic is already active. Ard's `unsafe` recovery boundary can recover such a panic like any other panic that crosses it.

### `try` is forbidden inside deferred work

`try` is not allowed in a deferred call expression or deferred block.

Deferred work is type-checked as the body of a zero-argument closure. That means ordinary closure-boundary rules apply: `break` cannot target loops outside the deferred work, and nested defers are scoped to the deferred closure. `try` gets a dedicated restriction because allowing it would be especially confusing: `try` in Ard propagates from the current function, but deferred work runs after the enclosing function is already exiting. Letting cleanup code silently replace, merge with, or disappear behind the original result would create surprising error behavior.

Rejected:

```ard
defer {
  try file.Close()
}
```

Use explicit matching instead. This restriction is lexical: functions called by deferred work may use `try` internally; the deferred work receives their `Result` or `Maybe` values like any other caller.

```ard
defer {
  match file.Close() {
    Result::err(e) => log::warn("close failed: {e}"),
    Result::ok(_) => (),
  }
}
```

### Placement

`defer` is valid only inside named function bodies, method bodies, closure literals, and script bodies. It is not valid at module scope.

`defer` is also not allowed inside module-level initializer expressions, even if the initializer contains a block-like expression. Such initializers lower outside a user-visible function boundary, so accepting `defer` there would make the lifetime unclear.

`defer` is not allowed inside `unsafe { ... }` blocks. The Go backend lowers unsafe blocks as helper IIFEs to install a `recover` defer. A user `defer` inside that IIFE would otherwise run at unsafe-block exit rather than the enclosing Ard function exit, violating this ADR's function-scoped guarantee. Move the `defer` outside the unsafe block or wrap the unsafe operation in a normal function.

## Lowering

The Go backend lowers each Ard defer to a Go `defer` whose operand is an immediately invoked zero-argument function literal.

Call form:

```ard
defer target(arg)
```

lowers to:

```go
defer func() {
    target(arg)
}()
```

Block form:

```ard
defer {
  stmt1
  stmt2
}
```

lowers to:

```go
defer func() {
    stmt1
    stmt2
}()
```

Because Go `defer` is function-scoped, existing lowering for `try`, `break`, loops, match arms, and ordinary block expressions can remain function-oriented. The backend does not need block-finalizer stacks or control-signal returns for this ADR.

The known exception is `unsafe { ... }`, which currently lowers as an IIFE for panic recovery. This ADR forbids `defer` inside unsafe blocks rather than requiring hoisting across that implementation boundary.

## Implementation notes

- Parser: add a `defer` keyword and statement node supporting call-expression and block forms.
- Checker:
  - require `defer` to appear inside a named function, method, closure literal, or script body;
  - reject `defer` at module scope, in module-level initializer expressions, and inside unsafe blocks;
  - require call-form defer to contain a call expression;
  - type-check deferred work in a void/discarded-value context;
  - reject `try` anywhere lexically inside deferred work.
- AIR: add a statement form for `defer` carrying either a call expression or a block.
- Go backend: emit `defer func() { ... }()` for both forms. Reuse the existing closure/body lowering path where possible so capture handling stays single-sourced instead of duplicating closure semantics for defer.
- Formatter/tree-sitter/Zed: add `defer` as a keyword and format/highlight both forms.
- Docs: document the divergence from Go's immediate argument evaluation.

## Consequences

- Ard gains a practical cleanup primitive for direct Go interop.
- The semantics are Go-compatible where it matters for resource lifetime: function-scoped registration, LIFO execution, and execution during early returns and panics.
- The evaluation model is more uniform than Go: deferred work runs later for both call and block forms.
- Block-scoped cleanup is intentionally not included. If needed later, it should be proposed as a separate `scope defer`-style feature or another distinct construct rather than changing `defer` semantics.
- Cleanup errors are explicit. Ignored cleanup results stay easy, and meaningful cleanup failures require an explicit `match` in the deferred block.
- `defer` becomes a reserved keyword. Existing code that uses `defer` as an identifier must rename it before upgrading.
- `defer` is available in scripts and runs at script-entry-function exit.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
