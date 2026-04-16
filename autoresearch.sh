#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPILER_DIR="$ROOT_DIR/compiler"
RUNS="${AR_RUNS:-2}"
WARMUP="${AR_WARMUP:-1}"
EXPORT_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$EXPORT_DIR"
}
trap cleanup EXIT

cd "$COMPILER_DIR"

go test -run '^$' ./bytecode/vm >/dev/null
./benchmarks/run.sh --runs "$RUNS" --warmup "$WARMUP" --export-dir "$EXPORT_DIR" >/tmp/ard-autoresearch-bench.out

python3 - <<'PY' "$EXPORT_DIR"
import json
import os
import sys

export_dir = sys.argv[1]
total_vm = 0.0
total_go = 0.0
vm_rows = []
for name in sorted(os.listdir(export_dir)):
    path = os.path.join(export_dir, name)
    with open(path, 'r', encoding='utf-8') as f:
        data = json.load(f)
    for result in data['results']:
        mean_ms = float(result['mean']) * 1000.0
        command = result['command']
        if command.startswith('vm:'):
            bench = command.split(':', 1)[1]
            vm_rows.append((bench, mean_ms))
            total_vm += mean_ms
        elif command.startswith('go:'):
            total_go += mean_ms

print(f"METRIC total_vm_ms={total_vm:.3f}")
print(f"METRIC total_go_ms={total_go:.3f}")
ratio = total_vm / total_go if total_go else 0.0
print(f"METRIC vm_go_ratio={ratio:.6f}")
for bench, mean_ms in vm_rows:
    print(f"METRIC vm_{bench}_ms={mean_ms:.3f}")
PY
