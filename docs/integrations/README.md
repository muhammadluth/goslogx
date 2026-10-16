# Optional integrations

Each plugin is a nested Go module. Installing core does not resolve framework, database or messaging client dependencies.

```bash
go get github.com/muhammadluth/goslogx
# After the prerequisite core and plugin releases are published:
go get github.com/muhammadluth/goslogx/plugins/http/fiber
go get github.com/muhammadluth/goslogx/plugins/http/std
go get github.com/muhammadluth/goslogx/plugins/database/bun
go get github.com/muhammadluth/goslogx/plugins/database/pg
go get github.com/muhammadluth/goslogx/plugins/cache/redis
```

| Module | Integration | Minimum Go |
| --- | --- | --- |
| [http/fiber](http/fiber.md) | Official Fiber v3 middleware | 1.26 |
| [http/std](http/std.md) | Official otelhttp handler/transport | 1.24 |
| [bun](database/bun.md) | Privacy-safe public QueryHook | 1.24 |
| [pg](database/pg.md) | go-pg v10 compatibility QueryHook | 1.24 |
| [grpc](grpc/README.md) | Official otelgrpc stats handlers | 1.24 |
| [redis](cache/redis.md) | Official redisotel hooks | 1.26 |

HTTP namespace adapters also include [Gin](http/gin.md), [Echo](http/echo.md), [Chi](http/chi.md), [Gorilla Mux](http/gorillamux.md), [httprouter](http/httprouter.md), [go-restful](http/gorestful.md) and [fasthttp](http/fasthttp.md). See the [HTTP integration guide](http/README.md) for installation paths, options and route/error behavior. The canonical standard HTTP path is `plugins/http/std`; Fiber uses `plugins/http/fiber`.

The expanded database and messaging modules below require Go 1.26. Bun and go-pg use the canonical database paths.

| Module below `plugins/` | Guide |
| --- | --- |
| `database/sql` | [Generic SQL and driver compatibility](database/sql.md) |
| `database/pgx` | [pgx v5](database/pgx.md) |
| `database/sqlx` | [sqlx](database/sqlx.md) |
| `database/gorm` | [GORM](database/gorm.md) |
| `database/ent` | [Ent](database/ent.md) |
| `database/mongo` | [MongoDB v2](database/mongo.md) |
| `database/clickhouse` | [ClickHouse](database/clickhouse.md) |
| `database/cassandra` | [Cassandra / ScyllaDB](database/cassandra.md) |
| `database/elasticsearch` | [Elasticsearch transport](database/elasticsearch.md) |
| `database/opensearch` | [OpenSearch transport](database/opensearch.md) |
| `database/neo4j` | [Neo4j](database/neo4j.md) |
| `database/dynamodb` | [DynamoDB](database/dynamodb.md) |
| `messaging/nats` | [NATS Core / JetStream](messaging/nats.md) |
| `messaging/kafka/franzgo` | [Kafka clients](messaging/kafka.md) |
| `messaging/kafka/kafkago` | [Kafka clients](messaging/kafka.md) |
| `messaging/kafka/sarama` | [Kafka clients](messaging/kafka.md) |
| `messaging/rabbitmq` | [RabbitMQ](messaging/rabbitmq.md) |

See [integration policy](../architecture/integration-policy.md) for statement sanitizers, bounded destination labels, propagation and choosing manual versus automatic instrumentation. Shared integration policies and testkits live in `internal/integration` within the core module. Plugins import those internal helpers only from this repository and consume the public `TelemetryProvider` contract. In a checkout, replace directives resolve sibling plugin modules.

Pass a `goslogx.TelemetryProvider`, such as the configured logger. Plugins use explicit tracer/meter providers and the propagator where relevant. Missing providers use no-ops; tracing-only and metrics-only configurations work. Plugins do not shut down providers or mutate OTel globals. Register instrumentation once, before concurrent application use; repeated registration adds duplicate hooks.

See [full observability](../../examples/full-observability/README.md) for Fiber, a Bun query, Redis, NATS, a worker and an outbound HTTP call in one trace. go-pg remains maintenance support; prefer Bun or another maintained driver for new projects.

## Integration priorities

Tier 1 includes net/http, Fiber, Gin, Echo, Chi, Gorilla Mux, SQL, pgx, Bun, GORM, MongoDB, Redis, NATS, franz-go and RabbitMQ. These cover the main server-to-database/cache/messaging paths.

Tier 2 includes httprouter, go-restful, fasthttp, go-pg, sqlx, Ent, ClickHouse, Elasticsearch, OpenSearch, kafka-go and IBM Sarama. Tier 3 includes Cassandra/Scylla, Neo4j, DynamoDB and driver-specific SQL compatibility. These adapters remain optional; use their individual guides for supported APIs and limits.

## Author a plugin

Accept this small consumer contract:

```go
type TelemetryProvider interface {
    TracerProvider() trace.TracerProvider
    MeterProvider() metric.MeterProvider
    TextMapPropagator() propagation.TextMapPropagator
}
```

Keep your target library in a separate module. Validate the target client, default missing capabilities to explicit no-ops, and configure upstream instrumentation with providers. Use the propagator only at network boundaries. Retain the caller's context and custom transport/client configuration.

```go
func Instrument(client *SomeClient, provider goslogx.TelemetryProvider, opts ...Option) error {
    if client == nil { return errors.New("myplugin: nil client") }
    // Resolve providers with explicit no-op defaults; do not read global providers.
    // Configure the target's official hooks using these providers and safe options.
    return nil
}
```

Use `type Option func(*config)` with matching concepts such as `WithDBName`, `WithRequestFilter` or `WithPropagator`. Use `github.com/your-org/goslogx-myplugin` as a stable scope. If wrapping upstream instrumentation, scope provider adapters can select the plugin scope while retaining upstream instrumentation versions. Follow current OTel semantic conventions. Bound operation/route/error dimensions, exclude secrets before collecting telemetry and let the application own provider lifecycles. Test with in-memory exporters/readers, assert parent relationships and privacy, and add your module to validation. Keep dependency versions compatible with the core and target library.
