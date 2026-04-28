# Autoresearch: Go backend decode_pipeline runtime

## Objective
Optimize the runtime speed of Ard's generated Go backend on `compiler/benchmarks/programs/decode_pipeline.ard`. This benchmark repeatedly parses a fixed JSON payload, decodes fields/lists/maps through `ard/decode`, and sums a checksum. Current runtime results showed generated Go (`go:decode_pipeline`) is much slower than VM and native Go, so focus on generated Go runtime overhead in decode-heavy code.

## Metrics
- **Primary**: go_decode_ms (ms, lower is better) — median execution time of the generated Go binary for `decode_pipeline`
- **Secondary**: build_ms, vm_output, go_output, binary_size_bytes — tradeoff/correctness monitors

## How to Run
`./autoresearch.sh` — rebuilds the compiler and generated Go binary, verifies output, then outputs `METRIC name=value` lines.

## Files in Scope
- `compiler/go/*.go` — Ard Go runtime helpers used by generated code, especially extern dispatch, decode builtins, Result/Maybe, map/list helpers.
- `compiler/go_backend/**/*.go` — Go backend lowering/rendering/optimizations if code generation changes are needed.
- `compiler/std_lib/decode.ard` — decode library definitions, only if a source-level change improves generated Go without regressing other backends.
- `compiler/benchmarks/programs/decode_pipeline.ard` only for temporary diagnostics; do not change the workload for kept experiments.
- `autoresearch.md`, `autoresearch.sh`, `autoresearch.checks.sh`, `autoresearch.ideas.md` — research bookkeeping and benchmark harness.

## Off Limits
- Do not weaken correctness checks or output verification.
- Do not change benchmark workload semantics or expected checksum to make results look better.
- Do not add new third-party dependencies.
- Do not optimize only by special-casing the benchmark program name/path.

## Constraints
- Generated Go output for `decode_pipeline.ard` must print `29678473`.
- Keep changes general-purpose and maintainable for Ard's Go backend/runtime.
- Passing benchmark runs are followed by `autoresearch.checks.sh`.
- Prefer small, explainable changes over large rewrites.

## What's Been Tried
- Baseline setup: generated Go decode_pipeline was previously around 402 ms under hyperfine. This harness measures direct generated binary runtime median in ms and should be used for apples-to-apples iteration.
- Source reading: generated decode code repeatedly calls `ardgo.CallExtern` for `DecodeInt`, `DynamicToList`, `DynamicToMap`, `ExtractField`, and `JsonToDynamic`; `CallExtern` uses an RWMutex-protected map lookup. Decode builtins also use reflection in `builtinDynamicToList`/`builtinDynamicToMap`, even though JSON values are usually `[]any` and `map[string]any`.
