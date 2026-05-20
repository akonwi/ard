# Decision: Retire the VM as a User-Facing Target

Status: accepted

## Context

Ard now has a production-capable Go target that supports:

- `ard run --target go`
- `ard build --target go`
- `ard test --target go`
- project-level Go FFI through `ffi.go` and `ffi/*.go`
- shared stdlib FFI contract coverage with the VM path

The bytecode VM was originally valuable as the primary execution runtime and as a path toward portable self-contained binaries. That role has changed. The Go target now provides the stronger deployment story: it is faster, compiles project FFI naturally, reuses Go tooling, and produces normal native binaries.

Keeping the VM as a public target creates unnecessary product and implementation complexity:

- `ard build --target vm` produces a slower binary than the Go target while still relying on Go compilation for packaging.
- Project Go FFI cannot run inside the stock `ard` interpreter process unless the project FFI code is compiled into a separate host runner.
- A temporary VM host runner for project FFI would be operationally similar to `ard run --target go`, but slower and more complex.
- Supporting both Go and VM as user-facing targets requires duplicate behavior decisions for testing, FFI, packaging, errors, and CLI defaults.

The VM may still have internal value as a compiler/runtime test backend while it exists. It exercises bytecode lowering, VM semantics, and runtime behavior independently from generated Go, but that does not justify exposing it as a normal user-facing target.

## Decision

Ard retires the VM as a user-facing CLI target. The Go target is the default native execution, build, and test target.

Concretely:

1. `ard run` uses the Go target by default.
2. `ard build` produces Go binaries by default.
3. `ard test` uses the Go target by default.
4. `ard run --target go`, `ard build --target go`, and `ard test --target go` remain supported explicit forms.
5. `ard build --target js-server` and `ard build --target js-browser` remain supported where applicable.
6. `ard run --target js-server` and `ard run --target js-browser` remain supported where applicable.
7. `vm` is no longer accepted by public CLI target parsing.
8. VM-specific coverage should live in compiler-internal Go tests such as `go test ./vm`, not public CLI workflows.

## Rationale

### Go is the production backend

The Go target is more performant and naturally supports project Go FFI because project companion files are compiled into the generated workspace. It also aligns compiled Ard artifacts with normal Go deployment expectations.

### VM project FFI would require a compiled host anyway

To support project Go FFI in `ard run --target vm`, the compiler would need to:

1. lower Ard to AIR/bytecode,
2. generate VM FFI adapters for project externs,
3. copy project `ffi.go` / `ffi/*.go`,
4. generate a small VM host runner,
5. compile that runner with the project FFI code,
6. execute the resulting temporary binary.

At that point the workflow is no longer a simple interpreter path. It has essentially the same operational shape as the Go target while retaining VM overhead and extra adapter machinery.

### Testing should match production

`ard test --target go` exists and exercises the same backend users run and ship. Making Go the default test target makes normal test results production-representative and allows project Go FFI tests to work without a special VM host-runner path.

## Non-goals

This decision does not immediately require deleting the VM implementation. It also does not require removing VM-focused internal tests.

This decision does not affect JavaScript targets. JS remains a separate backend family for browser/server JavaScript output.

## Implementation status

Implemented CLI changes:

- `backend.DefaultTarget` is `go`.
- public target parsing rejects `vm`.
- `ard run` has no VM branch.
- `ard build` has no VM branch and no embedded VM binary packaging path.
- `ard test` defaults to Go.
- VM-specific CLI helper code for embedded VM binaries was removed.

## Consequences

### Positive

- Simpler user-facing target model.
- Less duplicated FFI behavior.
- Production tests exercise the production backend by default.
- Avoids building a VM host-runner system with little user value.
- Clarifies Go as the primary native backend.

### Negative / tradeoffs

- VM-specific behavior is no longer visible through the CLI.
- Any remaining VM users need to migrate to Go or JS targets.
- Internal VM tests must be maintained deliberately if the VM remains in-tree.

## Follow-up work

- Update older bytecode roadmap docs that still describe VM binary packaging as the default build story.
- Decide later whether to keep VM as an internal conformance backend or delete the VM implementation entirely.
