#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR/compiler"

out_file="$(mktemp)"
trap 'rm -f "$out_file"' EXIT

if ! go test ./... >"$out_file" 2>&1; then
  tail -n 80 "$out_file"
  exit 1
fi
