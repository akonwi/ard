# Autoresearch: optimize Ard bytecode VM against the Go backend benchmark suite

## Objective
Improve the Ard bytecode VM on branch `perf/optimize-vm` so that runtime-mode benchmark performance moves materially closer to the Go backend introduced in PR #93, without changing benchmark programs, the benchmark harness semantics, or the Go backend implementation.

The goal is not to win a single microbenchmark. The goal is to reduce interpreter/runtime overhead across the representative benchmark corpus added in the PR: `sales_pipeline`, `shape_catalog`, `decode_pipeline`, `word_frequency_batch`, `async_batches`, `fs_batch`, and `sql_batch`.

## Metrics
- **Primary**: `total_vm_ms` (ms, lower is better) — sum of VM runtime means across the full runtime benchmark corpus.
- **Secondary**:
  - `total_go_ms` — Go backend runtime total for the same corpus.
  - `vm_go_ratio` — overall VM / Go runtime ratio.
  - `vm_<benchmark>_ms` for each benchmark — helps localize wins/regressions.

## How to Run
`./autoresearch.sh` — runs the runtime benchmark suite in `compiler/benchmarks/run.sh`, exports hyperfine JSON, and prints structured `METRIC ...` lines.

## Files in Scope
- `compiler/bytecode/vm/*.go` — interpreter loop, frame management, async runtime, module dispatch.
- `compiler/runtime/*.go` — object representation and helper operations used heavily by the VM.
- `compiler/bytecode/*.go` — only if bytecode-side metadata/layout changes are required for a real VM speedup.
- `autoresearch.md`, `autoresearch.sh`, `autoresearch.checks.sh`, `autoresearch.ideas.md` — experiment control files.

## Off Limits
- `compiler/benchmarks/programs/*.ard` — do not alter workloads.
- `compiler/benchmarks/run.sh` — do not weaken or narrow the benchmark harness to manufacture wins.
- `compiler/transpile/**` and Go backend code — comparison target only, not optimization target for this session.
- Any benchmark-specific shortcuts, constant folding cheats, cached outputs, or behavior changes that only help the measured programs.

## Constraints
- Do not overfit to the benchmark corpus.
- Do not cheat on the benchmarks.
- Preserve Ard semantics and existing test coverage.
- Run correctness checks after passing benchmark runs.
- Avoid committing generated binaries.
- Prefer changes that plausibly help many programs, not just one benchmark.

## Baseline
Initial runtime-suite sample on this branch (2 hyperfine runs per benchmark, warmup 1):
- `total_vm_ms` ≈ 971.791
- `total_go_ms` ≈ 586.571
- `vm_go_ratio` ≈ 1.656733

Largest current gaps are the pure/interpreter-heavy workloads:
- `sales_pipeline` ≈ 114.138 ms vs Go 7.104 ms
- `shape_catalog` ≈ 125.301 ms vs Go 6.749 ms
- `word_frequency_batch` ≈ 86.195 ms vs Go 9.351 ms
- `async_batches` ≈ 22.300 ms vs Go 4.218 ms

Closer-to-parity workloads:
- `decode_pipeline` ≈ 451.097 ms vs Go 405.402 ms
- `fs_batch` ≈ 109.248 ms vs Go 106.014 ms
- `sql_batch` ≈ 63.514 ms vs Go 47.734 ms

## What's Been Tried
- Session setup complete.
- Verified the benchmark harness from PR #93 and confirmed it compares prebuilt VM binaries against prebuilt Go-target binaries in runtime mode.
- Verified current tests pass on the starting branch.
- **Kept:** interned immutable bool objects and cached small integer objects in `compiler/runtime/object.go`. This reduced the suite from ~968.6 ms to ~851.7 ms (~12.1% faster), with especially strong gains in `sales_pipeline`, `shape_catalog`, `word_frequency_batch`, `async_batches`, and a meaningful gain in `decode_pipeline`.
- **Discarded:** preallocating frame stack slices. Too small to matter at suite level.
- **Discarded:** a function-name map for `lookupFunction`. It broke semantics because function names are not globally unique across modules.
- **Discarded:** checker composite type memoization via `sync.Map`. Slight microbenchmark improvement, but real-suite regressions from cache overhead.
- **Discarded:** inlining arithmetic/comparison fast paths directly in the VM loop. Some workloads improved, but not enough to beat the current best.
- **Discarded:** sharing primitive results from `Object.Copy()`. Did not help the full suite enough.
- Current working theory: broad wins come from reducing runtime object allocation churn with near-zero overhead, not from adding cache lookups or making the interpreter loop larger/more branchy.
- Next likely areas: more primitive/value constructor churn (especially strings/floats), collection/struct-heavy runtime helpers, and hot method paths used by the pure workloads.