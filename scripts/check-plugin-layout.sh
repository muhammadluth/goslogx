#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"

for old in plugins/bun plugins/pg plugins/fiber plugins/redis plugins/shared; do
  if [[ -e "$old" ]]; then
    printf 'Obsolete plugin directory remains: %s\n' "$old" >&2
    exit 1
  fi
done
for namespace in plugins plugins/http plugins/database plugins/cache plugins/messaging plugins/messaging/kafka; do
  if [[ -e "$namespace/go.mod" ]] || find "$namespace" -maxdepth 1 -name '*.go' -print -quit | rg -q .; then
    printf 'Integration package found in namespace directory: %s\n' "$namespace" >&2
    exit 1
  fi
done
if find internal/integration -name go.mod -print -quit | rg -q .; then
  printf 'Shared integration helpers must belong to the core module.\n' >&2
  exit 1
fi

expected=(
  plugins/http/{std,fiber,gin,echo,chi,gorillamux,httprouter,gorestful,fasthttp}
  plugins/grpc
  plugins/database/{sql,pgx,bun,pg,sqlx,gorm,ent,mongo,clickhouse,cassandra,elasticsearch,opensearch,neo4j,dynamodb}
  plugins/cache/redis
  plugins/messaging/nats
  plugins/messaging/kafka/{franzgo,kafkago,sarama}
  plugins/messaging/rabbitmq
)
for dir in "${expected[@]}"; do
  [[ -f "$dir/go.mod" ]] || { printf 'Missing plugin module: %s\n' "$dir" >&2; exit 1; }
  actual=$(sed -n 's/^module //p' "$dir/go.mod")
  wanted="github.com/muhammadluth/goslogx/$dir"
  [[ "$actual" == "$wanted" ]] || { printf 'Wrong module path in %s: %s\n' "$dir" "$actual" >&2; exit 1; }
done
count=$(find plugins -name go.mod | wc -l | tr -d ' ')
[[ "$count" == "${#expected[@]}" ]] || { printf 'Unexpected plugin modules: found %s, expected %s\n' "$count" "${#expected[@]}" >&2; exit 1; }
printf 'Canonical plugin layout verified: %s modules.\n' "$count"
