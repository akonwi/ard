# Benchmark program research for Ard JS targets

This note records external benchmark suites/repos that can inform Ard benchmark programs. The goal is not to copy a whole suite, but to choose small deterministic programs that stress compiler/runtime features and can verify VM/Go/JS parity.

## Sources surveyed

1. Computer Language Benchmarks Game
   - https://benchmarksgame-team.pages.debian.net/benchmarksgame/
   - Task descriptions: https://benchmarksgame-team.pages.debian.net/benchmarksgame/description/summary.html
   - Useful program families: `binary-trees`, `fannkuch-redux`, `fasta`, `k-nucleotide`, `mandelbrot`, `n-body`, `regex-redux`, `reverse-complement`, `spectral-norm`.
   - Takeaway for Ard: good source for classic CPU/string/allocation kernels with expected deterministic outputs.

2. hanabi1224/Programming-Language-Benchmarks
   - https://github.com/hanabi1224/Programming-Language-Benchmarks
   - Bench list observed under `bench/algorithm`: `binarytrees`, `coro-prime-sieve`, `edigits`, `fannkuch-redux`, `fasta`, `http-server`, `json-serde`, `knucleotide`, `lru`, `mandelbrot`, `merkletrees`, `nbody`, `nsieve`, `pidigits`, `regex-redux`, `secp256k1`, `spectral-norm`.
   - Takeaway for Ard: broader modernized CLBG-style set plus server/json/lru/crypto-ish workloads; useful for picking comparable algorithms with many language references.

3. kostya/benchmarks
   - https://github.com/kostya/benchmarks
   - README criteria emphasize idiomatic implementations, similar algorithms, and optimized/release artifacts.
   - Program families: `base64`, `brainfuck`, `json`, `matmul`, `primes`.
   - Takeaway for Ard: good examples of practical library-heavy benchmarks (`json`, `base64`) plus simple numeric kernels (`matmul`, `primes`).

4. WebKit JetStream
   - https://github.com/WebKit/JetStream
   - JS/WebAssembly benchmark suite with sub-suites such as `Octane`, `SunSpider`, `ARES-6`, `RexBench`.
   - Observed Octane examples: `crypto`, `deltablue`, `earley-boyer`, `navier-stokes`, `raytrace`, `regexp`, `richards`, `splay`, TypeScript compiler workload.
   - Takeaway for Ard: do not directly port large JS-specific suites now, but borrow workload shapes: object graphs, constraint solving, parser/compiler-like workloads, ray tracing, regexp/string processing.

5. Mozilla AreWeFastYet
   - https://github.com/mozilla/arewefastyet
   - Deprecated infrastructure, but benchmark inventory includes `SunSpider`, `ares6`, `asmjs`, `kraken`, `octane`, `six-speed`, `web-tooling-benchmark`.
   - Takeaway for Ard: confirms common JS-engine coverage themes: startup/code-load, object allocation, string/regex, typed numeric loops, parser/tooling workloads.

## Recommended additions derived from research

### High-priority additions

1. `numeric_kernel.ard`
   - Inspired by CLBG `spectral-norm`, `nbody`, `mandelbrot`, and kostya `matmul`.
   - Stress: integer/float arithmetic, division/modulo, nested loops, function calls.
   - JS value: catches numeric semantic differences and measures pure generated-code overhead.

2. `binary_trees.ard`
   - Inspired by CLBG/PLB `binarytrees`.
   - Stress: allocation, recursion or recursive-like traversal, structs, GC pressure.
   - JS value: object allocation and tree traversal are core server/browser workloads.

3. `k_nucleotide.ard` or `dna_frequency.ard`
   - Inspired by CLBG/PLB `knucleotide` and `fasta`.
   - Stress: string slicing/iteration, map counting, sorting top counts.
   - JS value: stronger string/map benchmark than current word-frequency text case.

4. `json_serde_roundtrip.ard`
   - Inspired by PLB `json-serde` and kostya `json`.
   - Stress: decode/encode/dynamic, nested lists/maps, Result-heavy validation.
   - JS value: directly exercises isomorphic prelude-backed JSON/dynamic behavior.

5. `lru_cache.ard`
   - Inspired by PLB `lru`.
   - Stress: maps, lists or linked-node structs, mutation-heavy workload.
   - JS value: realistic application data-structure workload; catches reference/mutation semantics.

6. `base64_batch.ard`
   - Inspired by kostya `base64`.
   - Stress: byte/string transformations, stdlib externs if using `ard/base64`.
   - JS value: good server/browser shared utility workload.

### Medium-priority additions

7. `prime_sieve.ard`
   - Inspired by PLB `nsieve`/`coro-prime-sieve` and kostya `primes`.
   - Stress: booleans/lists/nested loops; optional async variant later.

8. `raytrace_tiny.ard`
   - Inspired by Octane `raytrace`.
   - Stress: structs, methods, floats, polymorphic-ish object workflows.

9. `constraint_solver.ard`
   - Inspired by Octane `deltablue` / Richards-like scheduler workloads.
   - Stress: object graph mutation, dispatch-like control flow, maps/lists.

10. `parser_tooling.ard`
    - Inspired by JetStream/Web Tooling/TypeScript compiler workloads.
    - Stress: tokenization/parsing over strings, AST structs/unions, recursive descent.
    - JS value: closer to real compiler/transpiler workloads, but more implementation effort.

## Revised Ard benchmark suite shape

### Default timing suite

- Existing: `sales_pipeline`, `shape_catalog`, `decode_pipeline`, `word_frequency_batch`, `fs_batch`.
- Add: `numeric_kernel`, `binary_trees`, `json_serde_roundtrip`, `k_nucleotide`/`dna_frequency`, `lru_cache`.

### JS smoke/integration suite

- Build/run `js-server`: all default timing programs except target-specific unsupported ones.
- Build/import `js-browser`: pure/isomorphic programs only.
- Browser/mock-only: fetch/promise/http programs using deterministic `globalThis.fetch` mocks.

## Notes for adapting external benchmarks

- Prefer deterministic checksums over large printed output.
- Keep inputs embedded or generated deterministically; no network/random/wall-clock dependencies.
- Avoid overly tiny programs where startup dominates unless explicitly measuring startup.
- Keep native Go references idiomatic but semantically comparable.
- For browser JS smoke, avoid Node-only FFI (`fs`, `sql`) and use mocked browser APIs.
