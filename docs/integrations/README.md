# GOSLOGX INTEGRATIONS

goslogx integrations connect popular Go frameworks and clients to the same tracing, metrics, propagation, privacy, and lifecycle model without making those libraries dependencies of the core module.

Each integration is an optional Go module.

## INSTALLATION MODEL

```bash
go get github.com/muhammadluth/goslogx
```

Then install only what your application uses.

## HTTP INTEGRATIONS

See the [HTTP integration index](http/README.md) for installation and framework-specific usage.

```text
plugins/http/
├── std
├── fiber
├── gin
├── echo
├── chi
├── gorillamux
├── httprouter
├── gorestful
└── fasthttp
```

## GRPC INTEGRATION

Client/server instrumentation through explicit goslogx providers.

## DATABASE INTEGRATIONS

See the [database integration index](database/README.md) for implementation type, privacy, and construction guidance.

```text
plugins/database/
├── sql
├── pgx
├── bun
├── pg
├── sqlx
├── gorm
├── ent
├── mongo
├── clickhouse
├── cassandra
├── elasticsearch
├── opensearch
├── neo4j
└── dynamodb
```

## CACHE INTEGRATION

```text
plugins/cache/redis
```

## MESSAGING INTEGRATIONS

See the [messaging integration index](messaging/README.md) for producer, consumer, propagation, and delivery guidance.

```text
plugins/messaging/
├── nats
├── kafka/
│   ├── franzgo
│   ├── kafkago
│   └── sarama
└── rabbitmq
```

## PROVIDER MODEL

Plugins consume `goslogx.TelemetryProvider` and do not require global OpenTelemetry providers.

## CONTEXT CORRELATION

```text
HTTP SERVER
    ├── APPLICATION LOG
    ├── DATABASE
    ├── REDIS
    ├── OUTBOUND HTTP
    └── MESSAGE PRODUCER
           ↓
      REMOTE CONSUMER
           ↓
      WORKER DATABASE
```

## PRIVACY AND CARDINALITY

Avoid using trace IDs, span IDs, request IDs, transaction IDs, user IDs, raw URLs, query strings, SQL statements, Redis keys, message payloads, or raw error text as metric dimensions.

## AUTOMATIC INSTRUMENTATION

Preferred rule:

```text
MANUAL GOSLOGX PLUGIN
OR
AUTOMATIC INSTRUMENTATION
```

Using both can create duplicate spans and metrics.

## AUTHOR A PLUGIN

A third-party plugin should depend on the small public telemetry contract, preserve context, prefer maintained upstream OpenTelemetry instrumentation, use bounded dimensions, and keep secrets/payloads out by default.
