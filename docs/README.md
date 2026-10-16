# GOSLOGX DOCUMENTATION

Welcome to the goslogx documentation.

goslogx is a modular observability SDK for Go applications that combines structured logging, OpenTelemetry tracing, OpenTelemetry metrics, context propagation, trace/log correlation, privacy-aware instrumentation, and optional framework integrations.

Use this documentation to move from basic logging to full distributed observability without coupling application code to a specific telemetry backend.

## START HERE

| Goal | Recommended guide |
| --- | --- |
| Install and log your first event | [README](../README.md) |
| Enable traces and metrics | [OBSERVABILITY](guides/observability.md) |
| Create custom spans | [TRACING](guides/tracing.md) |
| Create custom metrics | [METRICS](guides/metrics.md) |
| Prepare for production | [PRODUCTION](guides/production.md) |
| Understand the architecture | [ARCHITECTURE](architecture/README.md) |
| Add framework integrations | [INTEGRATIONS](integrations/README.md) |
| Control cardinality and privacy | [CARDINALITY AND PRIVACY](guides/cardinality.md) |

## DOCUMENTATION MAP

```text
docs/
├── architecture/
├── guides/
└── integrations/
    ├── http/
    ├── grpc/
    ├── database/
    ├── cache/
    └── messaging/
```

## CORE GUIDES

### OBSERVABILITY

[OBSERVABILITY](guides/observability.md) explains service identity, managed versus application-owned providers, OTLP export, and the recommended initialization model.

### TRACING

[TRACING](guides/tracing.md) covers custom tracers, spans, attributes, events, status, errors, propagation, and trace/log correlation.

### METRICS

[METRICS](guides/metrics.md) covers managed metrics, custom meters, internal goslogx health metrics, exporter lifecycle, and application metric ownership.

### CARDINALITY AND PRIVACY

[CARDINALITY AND PRIVACY](guides/cardinality.md) documents safe metric dimensions, sensitive-data boundaries, and why identifiers such as user IDs, transaction IDs, Redis keys, and raw SQL must not become unbounded metric labels.

### PRODUCTION

[PRODUCTION](guides/production.md) covers initialization, export configuration, lifecycle ownership, graceful shutdown, and production operating concerns.

## INTEGRATIONS

### HTTP

net/http, Fiber, Gin, Echo, Chi, Gorilla Mux, httprouter, go-restful, and fasthttp.

### GRPC

Client and server instrumentation through explicit goslogx telemetry providers.

### DATABASE

database/sql, pgx, Bun, go-pg, sqlx, GORM, Ent, MongoDB, ClickHouse, Cassandra/ScyllaDB, Elasticsearch, OpenSearch, Neo4j, and DynamoDB.

### CACHE

go-redis.

### MESSAGING

NATS Core/JetStream, Kafka, and RabbitMQ.

## BACKEND STRATEGY

```text
APPLICATION
    ↓
GOSLOGX
    ↓
OPENTELEMETRY
    ↓
OTLP
    ↓
OPENTELEMETRY COLLECTOR
    ↓
GRAFANA / SIGNOZ / OTHER OTLP BACKENDS
```

## COMMERCIAL ADOPTION

goslogx is MIT licensed and can be used in commercial applications subject to the license terms.

For production adoption, review dependency compatibility, sampling, privacy, cardinality, Collector durability, backend retention, access control, and graceful shutdown.

## API REFERENCE

The public API reference is available on [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).
