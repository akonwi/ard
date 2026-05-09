# JavaScript/isomorphic benchmark suite

Goal: keep a small but representative benchmark corpus that can compare VM, Go, native Go, `js-server`, and browser-emitted JavaScript while also acting as robust JS target smoke coverage.

## Recommended benchmark set

### Keep from current corpus

| Program | Keep? | Why | JS target coverage |
| --- | --- | --- | --- |
| `sales_pipeline.ard` | yes | structs, enums, impl methods, maps, sorting, aggregation | `js-server`, `js-browser` import smoke |
| `shape_catalog.ard` | yes | union types, pattern matching, structs, list/map aggregation | `js-server`, `js-browser` import smoke |
| `decode_pipeline.ard` | yes | `ard/decode`, JSON parse, Result-heavy hot loop | `js-server`, `js-browser` import smoke |
| `word_frequency_batch.ard` | yes | string/list/map/sort hot path; caught duplicate local-name JS bug | `js-server`, `js-browser` import smoke |
| `fs_batch.ard` | yes | filesystem FFI and IO-heavy behavior | `js-server` only |
| `async_batches.ard` | keep, but not JS yet | fibers/concurrency comparison for VM/Go | skip JS until async semantics are real |
| `sql_batch.ard` | keep, but not JS | SQLite/database FFI | skip JS |

### Add targeted benchmark programs

1. `numeric_kernel.ard`
   - Purpose: arithmetic, integer division/modulo, float conversion, tight loops.
   - Why: catches numeric semantic drift and measures raw lowering/runtime overhead.
   - JS: `js-server` and `js-browser` import smoke.

2. `maybe_result_pipeline.ard`
   - Purpose: nested `Maybe`/`Result`, `.map`, `.and_then`, `try`, match on success/failure.
   - Why: AIR JS relies heavily on prelude wrappers here; performance and correctness matter.
   - JS: `js-server` and `js-browser` import smoke.

3. `module_cross_calls.ard` plus helper modules under a benchmark subdirectory.
   - Purpose: imported functions, imported struct/enum constructors, enum methods, type ownership.
   - Why: imported type constructor/export behavior was a major JS migration risk.
   - JS: `js-server` and `js-browser` import smoke.

4. `closure_sort_pipeline.ard`
   - Purpose: closure captures, anonymous functions, sort comparator calls, list transforms.
   - Why: generated JS closure/function identity and capture paths need pressure.
   - JS: `js-server` and `js-browser` import smoke.

5. `dynamic_json_roundtrip.ard`
   - Purpose: `ard/dynamic`, `ard/json`/`ard/encode`/`ard/decode` roundtrip through lists/maps/struct-like records.
   - Why: browser/server prelude-backed externs need identical behavior.
   - JS: `js-server` and `js-browser` import smoke.

6. `project_extern_adapter.ard`
   - Purpose: benchmark project-local JS FFI companion adapter calls with Maybe/Result/list/map returns.
   - Why: verifies `ffi.project.js-server.mjs` / `ffi.project.js-browser.mjs` wiring and adapters.
   - JS: both targets; runtime benchmark only if companion is deterministic and fast.

7. `browser_fetch_mock.ard`
   - Purpose: `ard/http`/fetch Promise path using a mocked `globalThis.fetch` in a JS runner.
   - Why: browser-only prelude/promise/fetch behavior needs coverage beyond source-shape tests.
   - JS: browser smoke/integration only; not part of timing comparison unless stable.

## Runner improvements

1. Split benchmark timing from JS target smoke tests.
   - Timing mode should remain small and stable.
   - JS smoke mode can build/import more programs and can include browser-target output that is not directly runnable by `ard run`.

2. Add `--smoke-js` mode:
   - Build every JS-supported benchmark as `js-server` and execute with Node.
   - Build every isomorphic benchmark as `js-browser` and execute/import with a browser-like Node harness when it has no DOM dependency.
   - For fetch/promise programs, install deterministic mocks before importing the generated browser module.

3. Export all benchmark results by default in CI/dev runs when requested:
   - `benchmarks/results/*.runtime.json`
   - optionally `benchmarks/results/*.cli.json`

4. Keep JS unsupported list explicit by reason:
   - `async_batches`: JS fibers currently synchronous/minimal.
   - `sql_batch`: no JS SQL stdlib target.
   - `fs_batch`: server-only FFI.

## Suggested default runtime suite

Use these for routine backend timing:

- `numeric_kernel`
- `sales_pipeline`
- `shape_catalog`
- `decode_pipeline`
- `word_frequency_batch`
- `maybe_result_pipeline`
- `fs_batch`

Run these less frequently or target-specific:

- `async_batches`
- `sql_batch`
- `project_extern_adapter`
- `browser_fetch_mock`

## Success criteria

- Every default timing benchmark verifies identical output for VM, Go, and supported JS targets.
- Every isomorphic benchmark builds for both `js-server` and `js-browser`.
- Browser-target output is imported/executed under a deterministic harness at least once in tests/CI.
- No benchmark depends on wall-clock time, network, random input, or machine-specific paths except isolated temp dirs.
