# 0024: Add Zig Target with Explicit Memory Semantics

## Status

Accepted

## Context

Ard currently behaves primarily as a Go transpiler. This has been productive, but Go-specific runtime behavior, especially around interfaces and host-backed implementations, increasingly constrains Ard's language and backend design.

Ard already has AIR as a backend boundary, so a new compiled target can reuse the existing parser, checker, module resolver, and AIR lowering. Zig is a good candidate for a native target because it can produce standalone binaries and exposes low-level representation choices directly. Zig also has no garbage collector, so an Ard-to-Zig backend must define memory management explicitly instead of accidentally inheriting Go runtime assumptions.

Two possible Zig approaches were considered:

- transpile AIR to Zig source
- lower AIR to bytecode and execute it with a Zig interpreter

A bytecode interpreter could be useful later for debugging, scripting, or portability, but it would require designing a VM, bytecode format, dynamic object model, dispatch loop, and host ABI before reaching parity with core language behavior.

## Decision

Add a `zig` target that transpiles AIR to Zig source.

The Zig target will pin Zig `0.16.0`. The compiler and CI should treat other Zig versions as unsupported unless a later ADR or implementation change updates the pinned version.

The Zig target should use Ard's existing backend pipeline:

```text
parse -> checker -> AIR -> Zig source -> Zig binary
```

The target name is `zig` for CLI target selection and `ard.toml` target configuration.

Ard `Int` lowers to Zig `i64`. Ard string behavior should preserve the current language behavior rather than introduce new Unicode indexing semantics as part of the Zig target.

Generated Zig code must use explicit memory management. The initial backend will use an arena-per-program or arena-per-test execution model. Generated functions that can allocate should receive or otherwise have access to a runtime context containing an allocator. The initial target does not promise fine-grained per-value destruction; allocated values are owned by the execution context and released when that context is destroyed.

The backend may also support a debug or test allocator mode, such as Zig's general-purpose allocator, for leak and misuse detection. That mode is for validation, not the initial language-level ownership model.

The first implementation should target core language parity and standard library modules that can be implemented with Zig's standard library and no external dependencies. Initial stdlib scope includes:

- `ard/argv`
- `ard/base64`
- `ard/chrono`
- `ard/dates`
- `ard/duration`
- `ard/env`
- `ard/float`
- `ard/fs`
- `ard/hex`
- `ard/int`
- `ard/io`
- `ard/list`
- `ard/map`
- `ard/string`
- `ard/testing`

`Maybe` and `Result` behavior are core language features and should be supported by the Zig runtime/lowering even though they are represented through standard library modules in user-facing code.

The first Zig target should defer:

- `Dynamic`
- JSON encode/decode and `ard/decode`
- async, fibers, wait groups, and channels
- HTTP
- SQL
- JavaScript-specific modules
- crypto functionality that requires external dependencies, such as password hashing

Future Zig extern support should extend the existing target-aware binding model with a `zig` binding key:

```ard
extern fn now() Int = {
  go = "Now"
  zig = "now"
}
```

Generated artifact behavior should match the other compiled targets: normal `build` output produces the requested binary. The compiler may preserve generated Zig sources through a debug flag or environment variable so backend failures can be inspected without making generated source directories the default user-facing artifact.

Implementation should proceed in stages:

1. Add `backend.TargetZig`, target parsing, target validation, and CLI routing for `run`, `build`, and `test`.
2. Create `compiler/zig` with source generation and build helpers.
3. Emit a small Zig runtime module with the execution context, allocator setup, strings, lists, maps, `Maybe`, `Result`, and test helpers.
4. Lower AIR primitives and simple control flow: constants, locals, globals, blocks, functions, calls, arithmetic, comparisons, `if`, `while`, and `break`.
5. Add parity tests that run the same Ard programs through Go and Zig and compare observable behavior.
6. Add structs, enums, field access, methods, lists, maps, strings, `Maybe`, `Result`, `try`, and pattern matching.
7. Add closures.
8. Add traits using explicit generated vtables rather than relying on Go-like interface behavior.
9. Implement the in-scope Zig standard library externs.
10. Add target-aware diagnostics for Zig-deferred modules and unsupported externs.

## Consequences

The Zig target avoids Go interface and runtime limitations while reusing Ard's existing compiler frontend and AIR boundary.

Memory management becomes an explicit backend contract. The initial arena model is simple and suitable for parity tests and short-lived programs, but long-running programs may retain memory until the execution context is destroyed. More precise ownership, lifetime analysis, or deterministic cleanup can be explored later without blocking the initial target.

Pinning Zig `0.16.0` reduces churn from Zig's evolving language and standard library, but the backend must be updated intentionally when Zig changes.

Deferring `Dynamic`, JSON, async, HTTP, SQL, JavaScript modules, and dependency-heavy crypto keeps the first Zig target focused on core language behavior instead of host integration breadth.

Generated Zig source should be readable enough for backend debugging, but Ard semantics remain defined by the checker and AIR, not by hand-editing generated code.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
