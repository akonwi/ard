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
- Baseline setup: generated Go decode_pipeline measured about 404 ms median with this harness (9 direct binary executions). Earlier hyperfine result was around 402 ms.
- Source reading: generated decode code repeatedly calls `ardgo.CallExtern` for `DecodeInt`, `DynamicToList`, `DynamicToMap`, `ExtractField`, and `JsonToDynamic`; `CallExtern` used an RWMutex-protected map lookup. Decode builtins also used reflection in `builtinDynamicToList`/`builtinDynamicToMap`, even though JSON values are usually `[]any` and `map[string]any`.
- Kept: removed the `ExternRegistry.Call` read lock for a tiny win (~401 ms). Risk: calls are no longer safe if registration races with execution; normal generated programs register before `Main`.
- Kept: fast-pathed `[]any`/`map[string]any` in Go decode builtins and avoided full map copy in `ExtractField` (~390 ms), then avoided copying dynamic lists/maps on the fast path (~383 ms). Risk: decoded list/map aliasing with underlying Dynamic values.
- Kept: direct switch fast paths for built-in decode extern names in `CallExtern` (~382 ms) and checking `float64` first in `builtinDecodeInt` (~381 ms). Both are small wins.
- Kept: Go backend emits direct `append` assignment / `map[key] = value` for ignored list push/map set results (~381 ms). Only statement calls are rewritten; expressions using return values still call helpers.
- Major kept win: `CoerceExtern` now avoids reflective `MethodByName`/`Method.Call` for source values whose type is `ardgo.Result[...]`, reading fields directly via the existing unsafe field-access pattern (~240 ms). This is the biggest improvement so far.
- Kept: `builtinDynamicToMap` now returns `Result[map[any]any, string]`, matching generated `[Dynamic:Dynamic]` and avoiding reflective map coercion (~231 ms).
- Kept: `MapKeys` detects all-string keys and uses `sort.Strings` before converting back to `[]K` (~230 ms).
- Discarded: broad generic `value.(T)` fast path at the start of `CoerceExtern`; it regressed slightly (~230.5 ms), likely because the extra assertion overhead did not pay off.
- Kept: `CoerceExtern` Result fast path now uses fixed field indexes instead of `FieldByName`, improving to ~189 ms. This assumes `Result` field order remains `value, err, ok`.
- Kept: Go backend emits direct typed helper calls for built-in decode extern wrappers (`DecodeString/Int/Float/Bool`, `DynamicToList/Map`, `ExtractField`, `JsonToDynamic`) instead of `CallExtern` + `CoerceExtern`, improving to ~45.7 ms. Then removed now-obsolete decode switch fast paths from `CallExtern` (~45.3 ms).
- Kept: empty list/map copy expressions now emit direct empty literals instead of `append([]T(nil), []T{}...)` (~45.0 ms).
- Kept: union type switches move `Dynamic`/`any` cases after concrete cases, fixing `Str | Dynamic` matching and improving `from_json` (~44.2 ms). This is also a semantic correctness improvement.
- Kept: decode extern helpers for string/int use direct Result literals and raw string/float64 fast paths before `builtinDynamicValue`, improving to ~41.3 ms.
- Discarded: restoring read lock in `ExternRegistry.Call` worsened the metric and increased binary size; keep the no-lock call path on this branch, but safety may need later design.
- Discarded: broad direct `Ok` literal rewrites inside decode builtins, direct `UnwrapOk` try success emission, raw fast paths for `DynamicToListExtern`/`DynamicToMapExtern`, and one-slice `MapKeys` sorting all regressed.
- Crashed: rewriting `std_lib/decode.ard` `match errors.size()` as if/else caused generated decode_pipeline nil dereference; do not retry without a focused compiler bug/regression test.
