# Ard backend benchmarks

This directory contains a small benchmark corpus for comparing:

- Ard's Go backend (`ard run` / `ard build`)
- handwritten idiomatic Go variants (`benchmarks/go/*`)

The benchmarks are intentionally more realistic than tiny microbenchmarks, but still self-contained and deterministic.

## Benchmark programs

- `numeric_kernel.ard`
  - integer/float arithmetic, nested loops, numeric semantics
- `binary_trees.ard`
  - object allocation, structs, recursive traversal
- `dna_frequency.ard`
  - string-heavy generated DNA data, map counting, string predicates
- `json_serde_roundtrip.ard`
  - JSON encode/decode, opaque values, Result-heavy hot loop
- `lru_cache.ard`
  - mutation-heavy map/list cache workload
- `base64_batch.ard`
- `fs_batch.ard`
  - directory lifecycle, repeated file write/read/copy/rename/delete
- `sql_batch.ard`
  - SQLite setup, transactional inserts, row query/decode, cleanup

## Requirements

Install [`hyperfine`](https://github.com/sharkdp/hyperfine).

For example with Homebrew:

```bash
brew install hyperfine
```

## Running benchmarks

From `compiler/`:

### Runtime-only comparison

Builds the Ard CLI once, then for each benchmark builds:

- one Ard Go backend binary
- one handwritten idiomatic Go binary

and benchmarks the resulting executables.

```bash
./benchmarks/run.sh
```

### End-to-end CLI comparison

Benchmarks the full `ard run` path instead of prebuilt binaries. The handwritten Go variant is included via `go run`:

```bash
./benchmarks/run.sh --mode cli
```

### Run a subset

```bash
./benchmarks/run.sh numeric_kernel json_serde_roundtrip
```

### Export hyperfine JSON results

```bash
./benchmarks/run.sh --export-dir benchmarks/results
```

## Notes

- `runtime` mode is the better apples-to-apples backend execution comparison.
- `cli` mode is useful if you want to include transpilation/build overhead in backend measurements.
- `native-go:*` command names refer to the handwritten idiomatic Go variants; `ard-go:*` command names refer to Ard's generated Go backend.
- Native Go variants are sanity-checked for output, but are allowed to differ when idiomatic Go semantics better capture the benchmark's intent than Ard implementation quirks.
- The runner builds the Ard CLI only once per invocation and reuses it across all benchmarks.
