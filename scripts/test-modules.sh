#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
modules=()
while IFS= read -r manifest; do
  module=${manifest#"$repo/"}
  module=${module%/go.mod}
  [[ "$manifest" != "$repo/go.mod" ]] || module=.
  modules+=("$module")
done < <(find "$repo" -name .git -prune -o -name go.mod -print | sort)
if (( $# > 0 )); then modules=("$@"); fi
workspaces=$(mktemp -d)
trap 'rm -rf "$workspaces"' EXIT
for module in "${modules[@]}"; do
  (
    cd "$repo/$module"
    echo "Validating module: $module"
    export GOWORK="$workspaces/${module//\//_}.work"
    version=$(python3 "$repo/scripts/module-workspace.py" "$module" "$GOWORK")
    export GOTOOLCHAIN="go${version}"
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
