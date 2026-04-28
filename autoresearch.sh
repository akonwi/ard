#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPILER_DIR="$ROOT_DIR/compiler"
TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR" "$COMPILER_DIR/benchmarks/programs/generated"
}
trap cleanup EXIT

ARD_BIN="$TMP_DIR/ard"
GO_BIN="$TMP_DIR/decode-go"
EXPECTED="29678473"
RUNS=9

start_ns() { python3 - <<'PY'
import time
print(time.perf_counter_ns())
PY
}

build_start=$(start_ns)
(cd "$COMPILER_DIR" && go build -tags=goexperiment.jsonv2 -o "$ARD_BIN" .)
(cd "$COMPILER_DIR" && "$ARD_BIN" build benchmarks/programs/decode_pipeline.ard --target go --out "$GO_BIN" >/dev/null)
build_end=$(start_ns)

output="$($GO_BIN)"
if [[ "$output" != "$EXPECTED" ]]; then
  echo "error: generated Go output mismatch: got '$output', expected '$EXPECTED'" >&2
  exit 1
fi

samples_file="$TMP_DIR/samples.txt"
python3 - "$GO_BIN" "$RUNS" > "$samples_file" <<'PY'
import subprocess, sys, time
bin_path = sys.argv[1]
runs = int(sys.argv[2])
for _ in range(runs):
    start = time.perf_counter_ns()
    out = subprocess.check_output([bin_path], text=True)
    end = time.perf_counter_ns()
    if out.strip() != "29678473":
        raise SystemExit(f"bad output: {out!r}")
    print((end - start) / 1_000_000)
PY

median_ms=$(python3 - "$samples_file" <<'PY'
import statistics, sys
with open(sys.argv[1]) as f:
    vals = [float(line) for line in f if line.strip()]
print(f"{statistics.median(vals):.6f}")
PY
)
build_ms=$(python3 - "$build_start" "$build_end" <<'PY'
import sys
print(f"{(int(sys.argv[2]) - int(sys.argv[1])) / 1_000_000:.6f}")
PY
)
binary_size=$(wc -c < "$GO_BIN" | tr -d ' ')

echo "METRIC go_decode_ms=$median_ms"
echo "METRIC build_ms=$build_ms"
echo "METRIC vm_output=$EXPECTED"
echo "METRIC go_output=$output"
echo "METRIC binary_size_bytes=$binary_size"
printf 'samples_ms='
paste -sd ' ' "$samples_file"
