# Ard backend benchmarks

This directory contains a small benchmark corpus for comparing:

- the default bytecode VM target (`ard run` / `ard build`)
- the Go backend (`ard run --target go` / `ard build --target go`)

The benchmarks are intentionally more realistic than tiny microbenchmarks, but still self-contained and deterministic.

## Benchmark programs

- `sales_pipeline.ard`
  - structs, enums, methods, lists, maps, sort, aggregation
- `shape_catalog.ard`
  - union types, pattern matching, structs, lists, maps
- `decode_pipeline.ard`
  - JSON decode, list/map decoders, result-heavy hot loop
- `word_frequency_batch.ard`
  - string-heavy list processing, map counting, sort
- `async_batches.ard`
  - async fibers, join, CPU-bound concurrent work
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
- one VM binary
- one Go-target binary
- one `js-server` module where supported

and benchmarks the resulting executables / runtime entrypoints.

```bash
./benchmarks/run.sh
```

### End-to-end CLI comparison

Benchmarks the full `ard run` / `ard run --target go` / `ard run --target js-server` path instead of prebuilt binaries:

```bash
./benchmarks/run.sh --mode cli
```

### Run a subset

```bash
./benchmarks/run.sh sales_pipeline decode_pipeline
```

### Export hyperfine JSON results

```bash
./benchmarks/run.sh --export-dir benchmarks/results
```

## Notes

- `runtime` mode is the better apples-to-apples backend execution comparison.
- `cli` mode is useful if you want to include transpilation/build overhead in backend measurements.
- `js-server` is included automatically for the currently supported benchmark subset:
  - `sales_pipeline`
  - `shape_catalog`
  - `decode_pipeline`
  - `word_frequency_batch`
  - `fs_batch`
- `async_batches` and `sql_batch` are currently skipped for `js-server` because `ard/async` and `ard/sql` are intentionally unsupported on JavaScript targets.
- The runner builds the Ard CLI only once per invocation and reuses it across all benchmarks.
