# JS Extern Binding Direction

This note captures the current design direction for Ard extern bindings as JavaScript support expands.

It is intentionally narrower than a full FFI spec. The goal is to lock in the shape of target-specific bindings and the JavaScript file lookup model before implementation work begins.

## Goals

- support real target-specific extern bindings
- avoid JS-specific emitter special cases for stdlib modules like `ard/io`
- keep user-facing Ard imports stable across targets where semantics are shared
- leave room for future user-defined JS interop, not just stdlib builtins
- avoid overfitting extern syntax to the current Go-only binding model

## Current Direction

The preferred direction is to extend the current `extern fn ... = ...` syntax with a multi-target binding block.

Example:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js-server = "readLine"
}

private extern fn _print(string: Str) Void = {
  go = "Print"
  js-server = "printLine"
}
```

This is preferred over introducing a JS-only module like `ard/io/js`.

Instead:

- `ard/io` stays the public module
- `ard/io` can be supported on `go`, `bytecode`, and `js-server`
- `ard/io` should remain unsupported on `js-browser`
- browser-specific functionality should later live in distinct browser-oriented modules rather than pretending browser IO is equivalent to server IO

## JavaScript FFI File Lookup

JavaScript extern bindings should not require a per-function module path in the Ard source.

Instead, the compiler should look for one companion FFI module per Ard module/target.

Preferred filenames:

- `ffi.js-server.mjs`
- `ffi.js-browser.mjs`

These files should export all JS extern implementations available for that Ard module on that target.

Conceptually:

- Ard module: `std_lib/io.ard`
- JS server companion: `std_lib/ffi.js-server.mjs`
- JS browser companion: `std_lib/ffi.js-browser.mjs`

The binding string in Ard then names the exported JS function, not the module path.

So:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js-server = "readLine"
}
```

means:

- call Go extern `ReadLine` on Go targets
- import and call `readLine` from the module's `ffi.js-server.mjs` on `js-server`

## Why this direction

### Better than JS emitter special-casing

This keeps stdlib modules like `ard/io`, `ard/float`, and `ard/int` on the same architectural path as later user-facing JS externs.

### Better than embedding JS module paths in every extern binding

Requiring every JS binding to spell out a module path is more verbose than necessary when the implementation naturally lives beside the Ard module.

### Better migration story from current syntax

Existing Go-only externs use:

```ard
extern fn read_line() Str!Str = "ReadLine"
```

Multi-target externs can grow into:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js-server = "readLine"
}
```

So codebases only adopt the expanded form when they need it.

## Future directives note

A future general directive system may still be desirable.

A lightweight syntax like this remains interesting for later language design:

```ard
#go("ReadLine")
#js_server("readLine")
extern fn read_line() Str!Str
```

This is worth keeping in mind if Ard later gains a broader directive/annotation system beyond externs.

However, for the current extern work, the binding-block form is preferred because it is a smaller extension of existing syntax and does not require committing to a general directive system yet.

## Open questions left for the full FFI design

- exact filesystem lookup rules for user modules vs stdlib modules
- whether missing target bindings should error eagerly at declaration time or when the function becomes reachable
- how JS extern imports are emitted and deduplicated
- how extern-bound JS exceptions map onto Ard `Result` vs panic behavior
- how `js-server` and `js-browser` differ when both bindings exist
- whether the old single-string form should remain as Go-only shorthand indefinitely or be desugared internally into the new multi-target representation
