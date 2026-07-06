# 0029: Remove JavaScript Targets

## Status

Accepted

## Context

Ard's JavaScript targets were experimental and added target selection, target-aware extern bindings, JavaScript runtime files, JavaScript standard-library companions, and backend-specific checker rules. That surface area now slows work on the Go implementation and the next FFI direction.

Ard's active execution path is the Go target. Keeping the JavaScript targets would require continued maintenance for type representation, stdlib parity, async semantics, and dependency-owned companions even though direct Go FFI work is the current priority.

## Decision

Remove the JavaScript targets entirely.

This includes:

- deleting the JavaScript backend and runtime files
- deleting JavaScript standard-library modules and FFI companion modules
- removing `--target` from `run`, `build`, and `test`
- removing `ard.toml` target resolution and checker target options
- removing JavaScript-specific stdlib import validation and union-match restrictions
- normalizing standard-library extern declarations to Go bindings

Extern binding blocks remain syntactically valid for explicit Go bindings:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
}
```

The shorthand form is equivalent and preferred:

```ard
extern fn read_line() Str!Str = "ReadLine"
```

Bindings for any target other than `go` are no longer supported.

## Consequences

- Ard has one supported execution backend: Go.
- The CLI no longer accepts target selection flags.
- Project manifests no longer configure a build target.
- Historical JavaScript ADRs are superseded and should not guide new implementation work.
- Future FFI work can focus on direct Go integration without preserving target-aware JavaScript abstractions.
