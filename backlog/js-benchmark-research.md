# Benchmark program research for future Ard JS target coverage

This note records external benchmark sources that can inform future Ard benchmark programs. The initial researched set has already been implemented in the default benchmark corpus (`numeric_kernel`, `binary_trees`, `dna_frequency`, `json_serde_roundtrip`, `lru_cache`, `base64_batch`).

## Sources surveyed

1. Computer Language Benchmarks Game
   - https://benchmarksgame-team.pages.debian.net/benchmarksgame/
   - Useful families: `binary-trees`, `fannkuch-redux`, `fasta`, `k-nucleotide`, `mandelbrot`, `n-body`, `regex-redux`, `reverse-complement`, `spectral-norm`.

2. hanabi1224/Programming-Language-Benchmarks
   - https://github.com/hanabi1224/Programming-Language-Benchmarks
   - Useful families: `binarytrees`, `coro-prime-sieve`, `json-serde`, `knucleotide`, `lru`, `mandelbrot`, `merkletrees`, `nbody`, `nsieve`, `regex-redux`, `spectral-norm`.

3. kostya/benchmarks
   - https://github.com/kostya/benchmarks
   - Useful families: `base64`, `brainfuck`, `json`, `matmul`, `primes`.

4. WebKit JetStream
   - https://github.com/WebKit/JetStream
   - Useful workload shapes: object graphs, constraint solving, parser/compiler-like workloads, ray tracing, regexp/string processing.

5. Mozilla AreWeFastYet benchmark inventory
   - https://github.com/mozilla/arewefastyet
   - Useful workload themes: startup/code-load, object allocation, string/regex, typed numeric loops, parser/tooling workloads.

## Adaptation notes

- Prefer deterministic checksums over large printed output.
- Keep inputs embedded or generated deterministically; no network/random/wall-clock dependencies.
- Avoid overly tiny programs where startup dominates unless explicitly measuring startup.
- Keep native Go references idiomatic but semantically comparable.
- For browser JS smoke, avoid Node-only FFI (`fs`, `sql`) and use mocked browser APIs.
