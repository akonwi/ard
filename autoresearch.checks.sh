#!/bin/bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/compiler"
go test ./go ./go_backend ./javascript ./bytecode/vm ./checker ./parse ./formatter 2>&1 | tail -80
