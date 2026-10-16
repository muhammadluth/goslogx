#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
dependencies=$(go list -deps .)
if forbidden=$(printf '%s\n' "$dependencies" | rg '^(github.com/(gofiber/|gin-gonic/gin|labstack/echo|go-chi/chi|gorilla/mux|julienschmidt/httprouter|emicklei/go-restful|valyala/fasthttp|uptrace/bun|go-pg/pg|jackc/pgx|redis/|nats-io/|twmb/franz-go|segmentio/kafka-go|IBM/sarama|rabbitmq/|ClickHouse/|apache/cassandra|neo4j/|aws/aws-sdk-go)|go.opentelemetry.io/contrib/instrumentation/|go.mongodb.org/|gorm.io/|entgo.io/|github.com/(elastic/go-elasticsearch|opensearch-project/))'); then
  printf 'Optional integration dependencies found in core:\n%s\n' "$forbidden" >&2
  exit 1
fi
printf 'Core dependency isolation verified.\n'

if rg --pcre2 -n '"github.com/muhammadluth/goslogx/internal/(?!integration/)' plugins --glob '*.go'; then
  printf 'Plugins may use shared internal/integration helpers, but not core telemetry internals.\n' >&2
  exit 1
fi
