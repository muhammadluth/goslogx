# goslogx documentation

Use goslogx for structured logging, OpenTelemetry traces and metrics, and optional framework integrations.

## Getting started

Start with [basic logging and installation](../README.md). Add telemetry with the [observability guide](guides/observability.md), then choose the integrations your application needs.

| Guide | Contents |
| --- | --- |
| [Observability](guides/observability.md) | Configuration, service resources and provider ownership |
| [Tracing](guides/tracing.md) | Custom tracers, spans, attributes and log correlation |
| [Metrics](guides/metrics.md) | Managed metrics, custom instruments and internal logger metrics |
| [Architecture](architecture/README.md) | Core and plugin boundaries, context flow and backend integration |
| [HTTP integrations](integrations/http/README.md) | Framework adapters, route templates, context and customization |
| [Plugins](integrations/README.md) | Installation, supported integrations and plugin authoring |
| [Integration privacy and coexistence](architecture/integration-policy.md) | Statement capture, messaging propagation and instrumentation layers |
| [Cardinality and privacy](guides/cardinality.md) | Safe attributes and sensitive-data handling |
| [Production](guides/production.md) | Initialization, export configuration and graceful shutdown |
| [Collector and infrastructure](guides/observability-production.md) | Deployment, external providers and infrastructure metric sources |

## Integrations

- [gRPC client and server](integrations/grpc/README.md)

- [Fiber v3](integrations/http/fiber.md)
- [net/http server and client](integrations/http/std.md)
- [Gin](integrations/http/gin.md), [Echo](integrations/http/echo.md), [Chi](integrations/http/chi.md), [Gorilla Mux](integrations/http/gorillamux.md)
- [httprouter](integrations/http/httprouter.md), [go-restful](integrations/http/gorestful.md), [fasthttp](integrations/http/fasthttp.md)
- [Bun](integrations/database/bun.md)
- [go-pg v10](integrations/database/pg.md)
- [go-redis v9](integrations/cache/redis.md)

Database integrations: [SQL](integrations/database/sql.md), [pgx](integrations/database/pgx.md), [sqlx](integrations/database/sqlx.md), [GORM](integrations/database/gorm.md), [Ent](integrations/database/ent.md), [MongoDB](integrations/database/mongo.md), [ClickHouse](integrations/database/clickhouse.md), [Cassandra](integrations/database/cassandra.md), [Elasticsearch](integrations/database/elasticsearch.md), [OpenSearch](integrations/database/opensearch.md), [Neo4j](integrations/database/neo4j.md) and [DynamoDB](integrations/database/dynamodb.md).

Messaging integrations: [NATS Core / JetStream](integrations/messaging/nats.md), [Kafka clients](integrations/messaging/kafka.md) and [RabbitMQ](integrations/messaging/rabbitmq.md).

The [full observability example](../examples/full-observability/README.md) connects Fiber, Bun, Redis, NATS, a remote worker and outbound HTTP in one trace.

Domain examples are grouped under [HTTP](../examples/http/README.md), [database SQL](../examples/database/sql/README.md), [cache Redis](../examples/cache/redis/README.md), and [messaging NATS](../examples/messaging/nats/README.md). The database, Kafka, and RabbitMQ folders also include focused setup snippets.
