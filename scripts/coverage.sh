#!/usr/bin/env bash
set -euo pipefail

# Run race-aware coverage for every Go module. Production modules must meet the
# configured minimum; examples are reported but are excluded by codecov.yml.
minimum="${COVERAGE_MINIMUM:-90}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
profiles="$(mktemp -d "${TMPDIR:-/tmp}/goslogx-coverage.XXXXXX")"
trap 'rm -rf "$profiles"' EXIT

status=0
printf '%-56s %8s\n' "module" "coverage"
printf '%-56s %8s\n' "------" "--------"

while IFS= read -r modfile; do
	module_dir="${modfile%/go.mod}"
	relative="${module_dir#./}"
	profile="$profiles/${relative//\//_}.out"
	(
		cd "$root/$module_dir"
		if [[ "$relative" == examples/* ]]; then
			go test -race -covermode=atomic -coverprofile="$profile" ./...
		else
			packages=()
			while IFS= read -r package; do
				packages+=("$package")
			done < <(go list ./... | grep -v '/examples/')
			go test -race -covermode=atomic -coverprofile="$profile" "${packages[@]}"
		fi
	)
	total="$(go tool cover -func="$profile" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')"
	printf '%-56s %7.1f%%\n' "$relative" "$total"
	if [[ "$relative" != examples/* ]] && awk -v total="$total" -v min="$minimum" 'BEGIN { exit !(total < min) }'; then
		echo "coverage below ${minimum}%: $relative (${total}%)" >&2
		status=1
	fi
done < <(cd "$root" && find . -name go.mod -not -path './vendor/*' | sort)

exit "$status"
