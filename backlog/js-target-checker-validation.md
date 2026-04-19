# JS Target Checker Validation

This document captures the current design for checker-side validation of stdlib module compatibility as Ard adds JavaScript targets.

## Summary

We want the compiler to reject imports of stdlib modules that are not portable to a given JS target.

This validation should happen in the checker/import path, not in JS codegen, so target incompatibilities fail early and transitively through user modules.

## Targets

The target set should become:

- `bytecode`
- `go`
- `js-browser`
- `js-server`

`js-browser` and `js-server` are intentionally separate targets because their available host APIs differ.

## Scope of Phase 1

Phase 1 should only cover:

- checker-side validation of stdlib imports
- module-level rules only
- transitive enforcement through user-module imports

Phase 1 should not cover:

- symbol-level portability rules
- new Ard syntax for portability annotations
- runtime FFI behavior
- `ard/http` portability modeling

## User-Facing Behavior

Target selection remains on:

- `ard run --target ...`
- `ard build --target ...`

`ard check` does **not** need a `--target` flag.

For checking that happens as part of `run` / `build`, the effective target is:

1. CLI `--target`, if provided
2. otherwise `ard.toml` `target`
3. otherwise the default target

## Checker API

The checker should receive the effective target explicitly.

Suggested API shape:

```go
type CheckOptions struct {
	Target string
}
```

And:

```go
c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{
	Target: target,
})
```

This should also be threaded through recursive checking of imported user modules so transitive validation uses the same target.

## Rule Representation

For now, use a simple allowlist of stdlib modules to allowed targets.

That is more intuitive than a capability system at this stage.

Suggested shape:

```go
var StdlibAllowedTargets = map[string]map[string]bool{
	"ard/fs": {
		"bytecode": true,
		"go":       true,
	},
	"ard/sql": {
		"bytecode": true,
		"go":       true,
	},
	"ard/env": {
		"bytecode":  true,
		"go":        true,
		"js-server": true,
	},
	"ard/argv": {
		"bytecode":  true,
		"go":        true,
		"js-server": true,
	},
}
```

Rule semantics:

- if a stdlib module is **not** listed in `StdlibAllowedTargets`, treat it as allowed for all targets for now
- if it **is** listed, the current target must appear in that module's allowlist

## Initial Restricted Modules

| Module     | bytecode | go | js-browser | js-server |
|------------|----------|----|------------|-----------|
| `ard/fs`   | yes      | yes | no         | no        |
| `ard/sql`  | yes      | yes | no         | no        |
| `ard/env`  | yes      | yes | no         | yes       |
| `ard/argv` | yes      | yes | no         | yes       |

Everything else remains unrestricted in phase 1.

## Why `ard/http` Is Deferred

`ard/http` likely mixes concerns that are not cleanly expressible as a single module-level rule.

Examples:

- client behavior vs server behavior
- browser-safe behavior vs server-only behavior

Because phase 1 only supports module-level allowlists, `ard/http` should be left out for now rather than modeled incorrectly.

## Enforcement Point

Validation should happen during stdlib import handling in the checker.

Conceptually:

1. checker sees a stdlib import
2. checker validates that the module is allowed for the effective target
3. if allowed, stdlib resolution continues normally
4. if not allowed, emit a checker error and continue collecting diagnostics

This ensures transitive behavior works automatically:

- if `main.ard` imports `my_app/db`
- and `my_app/db` imports `ard/sql`
- then `main.ard` fails under `js-browser`

## Diagnostic Shape

Suggested error format:

```text
Cannot import ard/sql when targeting js-browser; allowed targets: bytecode, go
```

This is explicit and actionable.

## Implementation Outline

### `compiler/backend/target.go`

Add:

- `js-browser`
- `js-server`

Update target parsing accordingly.

### `compiler/checker`

Add:

- `CheckOptions`
- a target-validation helper, e.g. `ValidateStdlibImportTarget(path, target)`
- stdlib allowed-target metadata, e.g. `StdlibAllowedTargets`

Update `checker.New(...)` and internal recursive `check(...)` to carry `CheckOptions`.

Update stdlib import handling to validate target compatibility before loading the module.

### Call Sites

Update checker construction in:

- `compiler/main.go`
- `compiler/transpile/transpile.go`
- relevant tests

## Tests to Add

### Target parsing

Add coverage for:

- `js-browser`
- `js-server`

### Checker import validation

Add tests for:

- `ard/env` allowed on `js-server`
- `ard/argv` allowed on `js-server`
- `ard/env` blocked on `js-browser`
- `ard/fs` blocked on `js-server`
- `ard/sql` blocked on `js-browser`
- unrestricted modules like `ard/io` still allowed on JS targets

### Transitive user-module validation

Add a project fixture where:

- a user module imports `ard/sql`
- the root module imports that user module
- checking under `js-browser` fails transitively

## Deferred Runtime / FFI Concern

There are a few target-blind embedded stdlib loads outside the main checker import flow, notably via `checker.FindEmbeddedModule(...)` in runtime FFI-related paths.

That is real architecture debt, but it is **not** the problem this phase is trying to solve.

Because those usages are largely runtime-oriented, they are not a reason to block checker-side stdlib target validation now.

This concern has been noted separately in `TODO.md` for later revisit when JS runtime/backend behavior becomes concrete.
