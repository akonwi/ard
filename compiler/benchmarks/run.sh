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
  "sales_pipeline:benchmarks/programs/sales_pipeline.ard"
  "shape_catalog:benchmarks/programs/shape_catalog.ard"
  "decode_pipeline:benchmarks/programs/decode_pipeline.ard"
  "word_frequency_batch:benchmarks/programs/word_frequency_batch.ard"
  "async_batches:benchmarks/programs/async_batches.ard"
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
  sales_pipeline
  shape_catalog
  decode_pipeline
  word_frequency_batch
  async_batches
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

supports_js_server() {
  case "$1" in
    sales_pipeline|shape_catalog|decode_pipeline|word_frequency_batch|fs_batch)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

native_go_program() {
  printf './benchmarks/go/%s\n' "$1"
}

cleanup_generated_program_dir() {
  local program_dir="${1%/*}"
  rm -rf "$ROOT_DIR/$program_dir/generated"
}

assert_same_output() {
  local benchmark="$1"
  local expected_name="$2"
  local expected="$3"
  local actual_name="$4"
  local actual="$5"

  if [[ "$actual" != "$expected" ]]; then
    echo "error: output mismatch for $benchmark: $actual_name differs from $expected_name" >&2
    echo "  $expected_name: $expected" >&2
    echo "  $actual_name: $actual" >&2
    exit 1
  fi
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
  local vm_bin="$tmp_dir/${name}-vm"
  local go_bin="$tmp_dir/${name}-go"
  local js_out="$tmp_dir/${name}.mjs"
  local js_runner="$tmp_dir/${name}-runner.mjs"
  local native_go_src
  native_go_src="$(native_go_program "$name")"
  local native_go_bin="$tmp_dir/${name}-native-go"

  cleanup_generated_program_dir "$rel_path"

  echo "==> building runtime binaries for $name"
  (cd "$ROOT_DIR" && "$ARD_BIN" build "$rel_path" --out "$vm_bin")
  (cd "$ROOT_DIR" && "$ARD_BIN" build "$rel_path" --target go --out "$go_bin")
  cleanup_generated_program_dir "$rel_path"
  (cd "$ROOT_DIR" && go build -o "$native_go_bin" "$native_go_src")

  local export_args=()
  if [[ -n "$EXPORT_DIR" ]]; then
    mkdir -p "$EXPORT_DIR"
    export_args+=(--export-json "$EXPORT_DIR/${name}.runtime.json")
  fi

  local commands=(
    --command-name "vm:$name" "$vm_bin"
    --command-name "go:$name" "$go_bin"
    --command-name "native-go:$name" "$native_go_bin"
  )

  if supports_js_server "$name"; then
    (cd "$ROOT_DIR" && "$ARD_BIN" build "$rel_path" --target js-server --out "$js_out")
    cat > "$js_runner" <<EOF
import { main } from "./$(basename "$js_out")";
main();
EOF
    commands+=(--command-name "js:$name" "node '$js_runner'")
  else
    echo "==> skipping js-server for $name (unsupported target/module set)"
  fi

  echo "==> verifying outputs for $name"
  local expected actual
  expected="$($vm_bin)"
  actual="$($go_bin)"
  assert_same_output "$name" "vm" "$expected" "go" "$actual"
  actual="$($native_go_bin)"
  assert_printed_output "$name" "native-go" "$actual"
  if supports_js_server "$name"; then
    actual="$(node "$js_runner")"
    assert_same_output "$name" "vm" "$expected" "js" "$actual"
  fi

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
  actual="$(cd "$ROOT_DIR" && "$ARD_BIN" run --target vm_next "$rel_path")"
  assert_same_output "$name" "vm" "$expected" "vm_next" "$actual"
  actual="$(cd "$ROOT_DIR" && "$ARD_BIN" run --target go "$rel_path")"
  assert_same_output "$name" "vm" "$expected" "go" "$actual"
  cleanup_generated_program_dir "$rel_path"
  actual="$(cd "$ROOT_DIR" && go run "$native_go_src")"
  assert_printed_output "$name" "native-go" "$actual"
  if supports_js_server "$name"; then
    actual="$(cd "$ROOT_DIR" && "$ARD_BIN" run --target js-server "$rel_path")"
    assert_same_output "$name" "vm" "$expected" "js" "$actual"
    cleanup_generated_program_dir "$rel_path"
  fi

  local commands=(
    --command-name "vm:$name" "cd '$ROOT_DIR' && '$ARD_BIN' run '$rel_path'"
    --command-name "vm_next:$name" "cd '$ROOT_DIR' && '$ARD_BIN' run --target vm_next '$rel_path'"
    --command-name "go:$name" "cd '$ROOT_DIR' && '$ARD_BIN' run --target go '$rel_path'"
    --command-name "native-go:$name" "cd '$ROOT_DIR' && go run '$native_go_src'"
  )

  if supports_js_server "$name"; then
    commands+=(--command-name "js:$name" "cd '$ROOT_DIR' && '$ARD_BIN' run --target js-server '$rel_path'")
  else
    echo "==> skipping js-server for $name (unsupported target/module set)"
  fi

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
