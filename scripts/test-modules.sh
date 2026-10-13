#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
modules=(. plugins/http plugins/fiber plugins/bun plugins/pg plugins/redis examples/full_observability)
if (( $# > 0 )); then modules=("$@"); fi
for module in "${modules[@]}"; do
  (
    cd "$repo/$module"
    echo "Validating module: $module"
    go mod tidy
    go vet ./...
    go test ./...
    go test -race ./...
    go test -cover ./...
    if [[ "${BENCHMARKS:-0}" == 1 ]]; then
      go test -run='^$' -bench=. -benchmem ./...
    fi
  )
done
