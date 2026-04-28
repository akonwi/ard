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
- Kept: decode extern helpers for string/int use direct Result literals and raw string/float64 fast paths before `builtinDynamicValue`, improving to ~41.3 ms. Later limited the raw int fast path to float64 (JSON numbers), improving slightly to ~41.2 ms.
- Discarded: restoring read lock in `ExternRegistry.Call` worsened the metric and increased binary size; keep the no-lock call path on this branch, but safety may need later design.
- Kept: `MapKeys` string fast path can return the sorted `[]string` directly when instantiated as `[]K == []string`, avoiding one copy for `map[string]V` loops (~41.24 ms before the float64-only int fast path).
- Discarded: broad direct `Ok` literal rewrites inside decode builtins, direct `UnwrapOk` try success emission, raw fast paths for `DynamicToListExtern`/`DynamicToMapExtern`, one-slice `MapKeys` sorting, `MapStringKeys` for for-in-map loops, JsonToDynamicExtern inlining, ExtractFieldExtern inlining, and scalar slow-helper splitting all regressed.
- Crashed: rewriting `std_lib/decode.ard` `match errors.size()` as if/else caused generated decode_pipeline nil dereference; do not retry without a focused compiler bug/regression test.
- Kept: pointer receivers for `Result.IsOk`/`IsErr` reduced generic Result copies in hot checks, improving to ~40.4 ms and shrinking binary. This may affect non-addressable public API calls, so review carefully.
- Kept: added pointer `Result.ExpectRef` and emit it for generated try/result-match success paths when the subject is addressable, improving to ~39.36 ms.
- Kept: `builtinDynamicValue` fast-paths common raw Dynamic types before checking `Encodable`, improving to ~41.1 ms before the Result pointer-receiver wins.
- Discarded: `decode::run` lowering inline, return block/if direct statement emission, `slices.Sort`, map[string]/map[any] MapKeys specializations, Result field reordering/exporting, manual `jsontext` parser, Result.Expect branch inversion, `UnwrapOkRef`, and `IsErr == false` all regressed.
- Kept: generated `ExpectRef` calls now rely on Go's implicit address-taking (`res.ExpectRef(...)`) instead of explicit `(&res).ExpectRef(...)`, improving to ~39.12 ms.
- Kept: `decode.map` now uses a string-key Dynamic map extern, with Go/JS/VM support. Go aliases raw `map[string]any` JSON objects instead of copying to `map[any]any`, improving to ~38.94 ms and then ~38.49 ms with a raw fast path in `DynamicToStringMapExtern`.
- Kept: Go backend direct-emits primitive/Void-to-Dynamic extern bodies, making `Dyn::from_str` a direct return instead of `CallExtern` in decode map key handling, improving to ~36.59 ms.
- Discarded after the map changes: raw `ExtractFieldExtern` fast path, direct Result literal in `DynamicToStringMapExtern`, limiting direct Dynamic conversion to only `StrToDynamic`, lowering dynamic constructors as IR intrinsics, small insertion sort for `MapKeys`, and no-arg `ExpectOkRef` all regressed.
- Kept: public `decode.string` and `decode.int` now delegate to scalar decode externs that return `[Error]` directly, avoiding the old `Error` result + try + `Result::ok` rewrap. Direct Result literals and split slow paths in those helpers improved to ~34.91 ms.
- Kept: `ffi.JsonToDynamic` uses an unsafe read-only byte-slice view of the input string to avoid allocating/copying `[]byte(jsonString)` before `json/v2.Unmarshal`, improving to ~35.00 ms before the scalar slow-path split.
- Discarded: generated-wrapper raw success duplication for scalar error-list externs, `Ok` constructor in the string error-list fast path, collapsed int check, removing unused `_decode_int`, top-level object-specific JSON unmarshal, and noinline slow helpers all regressed.
- Kept: `builtinExtractField` now handles raw `map[string]any` before `builtinDynamicValue`, improving field extraction slightly to ~34.87 ms; type-switch and direct-literal variants regressed.
- Kept: `DecodeIntErrorsExtern` uses a single-case `float64` type switch after the slow-path split, a tiny win (~34.86 ms); the analogous string/map/list type-switch attempts regressed.
- Kept: Go lowering specializes `decode.map(decode.string, value)` to a string-key map decoder, and further specializes `decode.map(decode.string, decode.int)` to a string-int decoder, improving to ~34.05 ms. These are general stdlib decode pattern specializations, not benchmark path/name checks.
- Kept: Go lowering specializes `decode.list(decode.int)` to an int-list decoder, then `DecodeIntListErrors` decodes raw JSON arrays directly in Go/JS/VM, improving to ~31.69 ms.
- Kept: `DecodeStringIntMapErrors` direct helper for string-int maps yielded a tiny improvement (~32.20 ms before the int-list raw array win); direct raw-map precheck inside that helper regressed.
- Discarded: nil slices for empty list literals, RegisterBuiltinExterns simple bool guard, MapKeys K=string zero-value specialization, direct raw map in string-int map helper, and type switch inside int-list loop all regressed.
- Kept: `DecodeIntListErrorsExtern` now fast-paths raw `[]any` arrays before `builtinDynamicToList`, improving to ~31.69 ms. Direct raw-map precheck in the string-int map helper did not help.
- Discarded after current best: returning named decoder functions from stdlib helpers (compile failure), retrying `decode.run` inlining (compile failure), direct field FFI helpers around specialized decoders, small string sorting networks, index-only/classic int-list loops, append-style int-list output, removing map capacity hints, `Ok`/`Err` final constructors, and nil error-slice checks all regressed.
- Kept after resuming: `MapKeys` pre-sizes the all-string key slice by length instead of using append, improving to ~31.64 ms. Caching `len(m)`, generic K=string specialization, and a four-key sorting network all regressed.
- Kept after resuming: direct int-list and direct string-int map decoders now fast-path raw Go `int` values after the JSON `float64` success path, improving to ~31.43 ms. Reordering int before float64, adding int64 branches, or switching to type switches all regressed.
- Discarded after resuming: direct Result literals for `JsonToDynamic`, encoding/json v1, slow-path helper splits in direct decoders, scalar `DecodeIntErrors`/`DecodeIntExtern` raw-int fast paths, and retrying raw map precheck in the direct string-int decoder all regressed.
- Kept: generated Go runtime now sets a default GC target of 300 during `RegisterBuiltinExterns` when the user has not set `GOGC`, improving allocation-heavy decode runtime to ~31.13 ms. Tested 150/250/350/400; all were worse than 300. Risk: higher default peak heap use, but users can override with `GOGC`.
- Kept: direct integer collection decoders call `decodeIntErrorsSlow` for fallback after owning their float64/int fast paths, avoiding redundant scalar fast-path code and improving to ~31.08 ms. Removing the raw int branches or splitting the fallback helper regressed.
- Kept: `DecodeIntListErrorsExtern` now fast-paths already-typed `[]int` inputs before JSON `[]any`, improving to ~31.01 ms. Checking `[]any` first, copying the `[]int`, or adding a `[]float64` fast path regressed. Risk: the kept typed-list path returns the original slice, so aliasing should be reviewed.
- Discarded: rechecking GC target 200 after the decoder changes still regressed; keep the default target at 300.
- Discarded: rechecking `slices.Sort`, using a top-level type switch for direct int-list inputs, and binding non-addressable `result_expect` calls to temporary variables all regressed.
- Kept: after later runtime changes, `DynamicToStringMapExtern` direct raw `map[string]any` Result literal became profitable, improving to ~30.93 ms and shrinking the binary. Rechecking direct literals in `ExtractField`, `JsonToDynamic`, `builtinDynamicToStringMap`, and `builtinDynamicToList` still regressed.
- Discarded: raw `DynamicToListExtern` fast path, ok-first branch layout in string-int map conversion, fixed-array four-key `MapKeys`, ignored-key `MapValues` lowering, cap-limited typed int-list return, GC target 275/325, and moving GC tuning to package init all regressed.
- Lazy JSON Dynamic prototype: returning lazy JSON and decoding requested fields/lists/maps on demand improved dramatically (~23.5 ms) when validated with `encoding/json.Valid`, but was discarded because that validator accepts duplicate object names that json/v2 rejects. A jsontext-compatible validation version preserved semantics but regressed (~31.24 ms). Kept version uses `encoding/json.Valid` plus a custom duplicate-name scanner to preserve json/v2 duplicate rejection, improving to ~29.32 ms, with tests for duplicate names, requested fields, and generic fallback.
- Kept: lazy JSON integer parsing now uses a manual digit parser with `ParseFloat` fallback, improving to ~25.41 ms. Lazy int-list initial capacity increased to 12, improving to ~25.03 ms; cap 10 and 24 regressed. Pre-counting int-list elements or map members also regressed.
- Kept: lazy JSON scanner avoids `strconv.Unquote` for simple unescaped object keys, improving to ~22.88 ms. Duplicate-name validation now tracks names with a fixed 16-slot array before map fallback, improving to ~22.33 ms; 4 slots regressed.
- Kept: lazy JSON integer parsing now parses directly from the original JSON string and returns the end offset, avoiding a separate number-boundary scan and improving to ~20.80 ms. Removing the now-unused older helper regressed slightly/noisily.
- Kept: small top-level lazy JSON objects are cached in a fixed field array so repeated field extraction avoids rescans, improving to ~20.15 ms with 8 slots and ~19.22 ms with 3 slots. Two slots regressed; 3 slots is fast but high overfit risk for broader objects.
- Kept: lazy string-int maps use a fixed capacity hint of 4, improving slightly to ~19.18 ms. Capacity 2 and 8 regressed.
- Kept: lazy JSON validation now checks duplicates before `json.Valid`, improving slightly to ~19.13 ms. Rechecking generic K=string `MapKeys` specialization after lazy JSON still regressed.
- Discarded after lazy JSON: GC targets 100/200/500 all regressed; keep 300. Direct Result literals for cached lazy field extraction also regressed.
- Kept: cached lazy JSON objects are now stored by pointer to avoid copying fixed key/value arrays through Dynamic, improving to ~18.98 ms. Constructing the pointer directly improved slightly to ~18.94 ms. Four-slot pointer cache regressed; three slots remains fastest but has overfit risk.
- Discarded: filling lazy int-list output by index into a len-12 slice regressed; keep cap-12 append shape.
