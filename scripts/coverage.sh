#!/usr/bin/env bash
set -euo pipefail

# Run race-aware coverage for every Go module. Production modules must meet the
# configured minimum; examples are reported but are excluded by codecov.yml.
minimum="${COVERAGE_MINIMUM:-90}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/goslogx-coverage.XXXXXX")"
profiles="${COVERAGE_OUTPUT_DIR:-$temporary}"
mkdir -p "$profiles"
profiles="$(cd "$profiles" && pwd)"
trap 'rm -rf "$temporary"' EXIT

printf '%-56s %8s\n' "module" "coverage"
printf '%-56s %8s\n' "------" "--------"

while IFS= read -r modfile; do
	module_dir="${modfile%/go.mod}"
	relative="${module_dir#./}"
	profile="$profiles/${relative//\//_}.out"
	workspace="$temporary/${relative//\//_}.work"
	version=$(python3 "$root/scripts/module-workspace.py" "$relative" "$workspace")
	export GOWORK="$workspace" GOTOOLCHAIN="go${version}"
	(
		cd "$root/$module_dir"
		if [[ "$relative" == examples/* ]]; then
			go test -race -coverpkg=github.com/muhammadluth/goslogx/... -covermode=atomic -coverprofile="$profile" ./...
		else
			packages=()
			while IFS= read -r package; do
				packages+=("$package")
			done < <(go list ./... | grep -v '/examples/')
			go test -race -coverpkg=github.com/muhammadluth/goslogx/... -covermode=atomic -coverprofile="$profile" "${packages[@]}"
		fi
	)
done < <(cd "$root" && find . -name go.mod -not -path './vendor/*' | sort)

python3 "$root/scripts/coverage-report.py" "$profiles" --minimum "$minimum"
