# Go Backend Notes (Optional)

This is not the primary path, but it is a viable follow-up once bytecode exists.

## Key Realities

- Go codegen will still embed runtime.Object, Result, Maybe, Dynamic.
- The async runtime should stay in Ard runtime code; do not map to goroutines directly unless semantics match.
- FFI registry needs to be available or replaced by direct bindings.

## Minimal Approach

- Emit Go code that calls runtime helpers for every operation.
- Package the runtime as a library imported by generated code.
- Treat codegen as "lowered IR to Go" with minimal semantic shifts.

## Risks

- Divergent behavior if Go semantics are used directly (e.g., numeric conversions, string formatting).
- Closure capture and mutability semantics are easy to get wrong without a runtime wrapper.
