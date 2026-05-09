# Remaining JavaScript benchmark/smoke work

The default benchmark corpus has been replaced with JS-focused workloads:

- `numeric_kernel`
- `binary_trees`
- `dna_frequency`
- `json_serde_roundtrip`
- `lru_cache`
- `base64_batch`
- `fs_batch`
- `sql_batch` (non-JS)

This file tracks only remaining benchmark and JS-target hardening work.

## Remaining work

1. Add a dedicated JS smoke mode to `compiler/benchmarks/run.sh`.
   - Proposed flag: `--smoke-js`.
   - Build every JS-supported benchmark as `js-server` and execute with Node.
   - Build every isomorphic benchmark as `js-browser` and import/execute with a Node/browser-like harness.
   - Keep timing mode separate from smoke mode.

2. Add browser-target runtime smoke coverage.
   - Current benchmarks validate `js-server` runtime behavior.
   - `js-browser` builds should also be imported/executed where no DOM-only APIs are required.
   - For browser-only APIs, install deterministic mocks before importing generated modules.

3. Add targeted browser/mock integration programs.
   - `browser_fetch_mock`: exercise `ard/http`/fetch Promise path with mocked `globalThis.fetch`.
   - `project_extern_adapter`: exercise project-local JS FFI companions for both `js-server` and `js-browser`.

4. Keep JS unsupported targets explicit by reason.
   - `sql_batch`: no JS SQL stdlib target.
   - Any future async benchmark: JS fibers are currently synchronous/minimal, not true concurrency.

## Success criteria

- Every default timing benchmark verifies identical output for VM, Go, and supported JS targets.
- Every isomorphic benchmark builds for both `js-server` and `js-browser`.
- Browser-target output is imported/executed under a deterministic harness at least once in tests or benchmark smoke mode.
- No benchmark depends on wall-clock time, network, random input, or machine-specific paths except isolated temp dirs.
