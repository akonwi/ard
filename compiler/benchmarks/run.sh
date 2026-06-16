#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARD_BIN="$ROOT_DIR/ard-bench"
BENCH_TMP_DIR=""
MODE="runtime"
RUNS=10
WARMUP=2
EXPORT_DIR=""

BENCHMARKS=(
  "numeric_kernel:benchmarks/programs/numeric_kernel.ard"
  "binary_trees:benchmarks/programs/binary_trees.ard"
  "dna_frequency:benchmarks/programs/dna_frequency.ard"
  "json_serde_roundtrip:benchmarks/programs/json_serde_roundtrip.ard"
  "lru_cache:benchmarks/programs/lru_cache.ard"
  "base64_batch:benchmarks/programs/base64_batch.ard"
  "fs_batch:benchmarks/programs/fs_batch.ard"
  "sql_batch:benchmarks/programs/sql_batch.ard"
)

usage() {
  cat <<'EOF'
Usage: benchmarks/run.sh [options] [benchmark-name ...]

Options:
  --mode runtime|cli   Benchmark built binaries or `ard run` commands (default: runtime)
                       Runtime mode also compares prebuilt idiomatic Go binaries;
                       CLI mode compares `go run` for the idiomatic Go variants.
  --runs N             Hyperfine run count (default: 10)
  --warmup N           Hyperfine warmup runs (default: 2)
  --export-dir PATH    Export per-benchmark hyperfine JSON into PATH
  -h, --help           Show this help

Available benchmarks:
  numeric_kernel
  binary_trees
  dna_frequency
  json_serde_roundtrip
  lru_cache
  base64_batch
  fs_batch
  sql_batch
EOF
}

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required tool '$1' not found" >&2
    exit 1
  fi
}

build_ard() {
  echo "==> building Ard CLI once: $ARD_BIN"
  (cd "$ROOT_DIR" && go build -tags=goexperiment.jsonv2 -o "$ARD_BIN" .)
}

lookup_program() {
  local wanted="$1"
  local entry name path
  for entry in "${BENCHMARKS[@]}"; do
    name="${entry%%:*}"
    path="${entry#*:}"
    if [[ "$name" == "$wanted" ]]; then
      printf '%s\n' "$path"
      return 0
    fi
  done
  return 1
}


native_go_program() {
  printf './benchmarks/go/%s\n' "$1"
}

cleanup_generated_program_dir() {
  local program_dir="${1%/*}"
  rm -rf "$ROOT_DIR/$program_dir/generated"
}

assert_printed_output() {
  local benchmark="$1"
  local command_name="$2"
  local output="$3"

  if [[ -z "$output" ]]; then
    echo "error: $command_name:$benchmark produced no output" >&2
    exit 1
  fi
}

selected_benchmarks() {
  if [[ "$#" -eq 0 ]]; then
    for entry in "${BENCHMARKS[@]}"; do
      printf '%s\n' "${entry%%:*}"
    done
    return 0
  fi

  local name
  for name in "$@"; do
    if ! lookup_program "$name" >/dev/null; then
      echo "error: unknown benchmark '$name'" >&2
      exit 1
    fi
    printf '%s\n' "$name"
  done
}

run_runtime_benchmark() {
  local name="$1"
  local rel_path="$2"
  local tmp_dir="$3"
  local ard_go_bin="$tmp_dir/${name}-ard-go"
  local native_go_src
  native_go_src="$(native_go_program "$name")"
  local native_go_bin="$tmp_dir/${name}-native-go"

  cleanup_generated_program_dir "$rel_path"

  echo "==> building runtime binaries for $name"
  (cd "$ROOT_DIR" && "$ARD_BIN" build "$rel_path" --out "$ard_go_bin")
  cleanup_generated_program_dir "$rel_path"
  (cd "$ROOT_DIR" && go build -o "$native_go_bin" "$native_go_src")

  local export_args=()
  if [[ -n "$EXPORT_DIR" ]]; then
    mkdir -p "$EXPORT_DIR"
    export_args+=(--export-json "$EXPORT_DIR/${name}.runtime.json")
  fi

  local commands=(
    --command-name "ard-go:$name" "$ard_go_bin"
    --command-name "native-go:$name" "$native_go_bin"
  )

  echo "==> verifying outputs for $name"
  local expected actual
  expected="$($ard_go_bin)"
  assert_printed_output "$name" "ard-go" "$expected"
  actual="$($native_go_bin)"
  assert_printed_output "$name" "native-go" "$actual"

  hyperfine \
    --warmup "$WARMUP" \
    --runs "$RUNS" \
    "${commands[@]}" \
    "${export_args[@]}"

  cleanup_generated_program_dir "$rel_path"
}

run_cli_benchmark() {
  local name="$1"
  local rel_path="$2"

  local export_args=()
  if [[ -n "$EXPORT_DIR" ]]; then
    mkdir -p "$EXPORT_DIR"
    export_args+=(--export-json "$EXPORT_DIR/${name}.cli.json")
  fi

  local native_go_src
  native_go_src="$(native_go_program "$name")"

  cleanup_generated_program_dir "$rel_path"

  echo "==> verifying outputs for $name"
  local expected actual
  expected="$(cd "$ROOT_DIR" && "$ARD_BIN" run "$rel_path")"
  assert_printed_output "$name" "ard-go" "$expected"
  actual="$(cd "$ROOT_DIR" && go run "$native_go_src")"
  assert_printed_output "$name" "native-go" "$actual"

  local commands=(
    --command-name "ard-go:$name" "cd '$ROOT_DIR' && '$ARD_BIN' run '$rel_path'"
    --command-name "native-go:$name" "cd '$ROOT_DIR' && go run '$native_go_src'"
  )

  hyperfine \
    --warmup "$WARMUP" \
    --runs "$RUNS" \
    "${commands[@]}" \
    "${export_args[@]}"

  cleanup_generated_program_dir "$rel_path"
}

main() {
  require_tool hyperfine
  local names=()
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --mode)
        MODE="$2"
        shift 2
        ;;
      --runs)
        RUNS="$2"
        shift 2
        ;;
      --warmup)
        WARMUP="$2"
        shift 2
        ;;
      --export-dir)
        EXPORT_DIR="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        names+=("$1")
        shift
        ;;
    esac
  done

  if [[ "$MODE" != "runtime" && "$MODE" != "cli" ]]; then
    echo "error: --mode must be 'runtime' or 'cli'" >&2
    exit 1
  fi

  build_ard

  BENCH_TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$BENCH_TMP_DIR" "$ARD_BIN"' EXIT

  local name rel_path
  while IFS= read -r name; do
    rel_path="$(lookup_program "$name")"
    if [[ "$MODE" == "runtime" ]]; then
      run_runtime_benchmark "$name" "$rel_path" "$BENCH_TMP_DIR"
    else
      run_cli_benchmark "$name" "$rel_path"
    fi
    echo
  done < <(selected_benchmarks "${names[@]}")
}

main "$@"
