# GOSLOGX

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/graph/badge.svg?token=QD1YFY5MC8)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**A MODULAR OBSERVABILITY SDK FOR PRODUCTION GO APPLICATIONS.**

goslogx combines structured logging, sensitive-data masking, OpenTelemetry tracing, OpenTelemetry metrics, trace/log correlation, and optional framework integrations behind one consistent Go API.

It is designed for teams that want production observability without coupling application code to a specific backend or forcing every framework, database, cache, and messaging dependency into the core module.

```text
APPLICATION
    │
    ├── LOGS
    ├── TRACES
    ├── METRICS
    └── CONTEXT PROPAGATION
            │
            ▼
         GOSLOGX
            │
            ▼
     OPENTELEMETRY / OTLP
            │
            ▼
   OPENTELEMETRY COLLECTOR
            │
      ┌─────┼──────────────┐
      ▼     ▼              ▼
   GRAFANA  SIGNOZ       OTHER OTLP BACKENDS
```

## WHY GOSLOGX

goslogx is built around a few deliberate principles:

- **BACKEND AGNOSTIC** — emit OpenTelemetry and OTLP instead of binding your application to one observability vendor.
- **MODULAR BY DEFAULT** — install only the integrations your application actually uses.
- **PRODUCTION-ORIENTED** — provider ownership, batching, sampling, graceful shutdown, masking, privacy defaults, and bounded-cardinality guidance are first-class concerns.
- **CONTEXT-AWARE** — correlate logs with active spans and preserve trace context across HTTP, databases, Redis, messaging, and workers.
- **LOW COUPLING** — the core module does not import your web framework, ORM, cache client, or broker.
- **EXTENSIBLE** — integrations are optional Go modules built around a small `TelemetryProvider` contract.

## FEATURE HIGHLIGHTS

| Capability | What goslogx provides |
| --- | --- |
| Structured logging | Zap-based JSON logging with consistent severity and message types |
| Sensitive-data protection | Struct tags, JSON masking, HTTP header masking, privacy-safe defaults |
| Distributed tracing | Managed or external OpenTelemetry `TracerProvider`, OTLP/gRPC export, sampling and batching |
| Metrics | Managed or external `MeterProvider`, custom meters and internal goslogx health metrics |
| Correlation | `obs_trace_id`, `obs_span_id`, and `obs_trace_flags` on context-aware logs |
| Propagation | W3C Trace Context and pluggable propagators |
| Lifecycle | Unified `ForceFlush` and `Shutdown` for goslogx-owned telemetry |
| Integrations | HTTP, gRPC, databases, Redis, NATS, Kafka, RabbitMQ, and more |
| Commercial use | MIT licensed; suitable for commercial applications subject to the license terms |

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx
```

Core requires Go 1.24 or later. Some newer plugin modules require Go 1.26. Each plugin is a separate Go module, so installing core does not pull framework, database, cache, or messaging dependencies.

## QUICK START

```go
package main

import "github.com/muhammadluth/goslogx"

func main() {
    logger := goslogx.New(
        goslogx.WithServiceName("payment-api"),
        goslogx.WithMasking(true),
    )

    logger.Info(
        "",
        "payments",
        goslogx.MESSSAGE_TYPE_EVENT,
        "payment received",
        map[string]any{"result": "accepted"},
    )
}
```

`New` keeps the existing singleton behavior: the first call configures the global logger, while package-level helpers reuse the same instance.

## SENSITIVE-DATA MASKING

```go
type PaymentEvent struct {
    Password string         `json:"password" log:"masked:full"`
    Email    string         `json:"email" log:"masked:partial"`
    Payload  map[string]any `json:"payload" log:"masked:auto"`
}
```

| Tag | Behavior |
| --- | --- |
| `log:"masked:full"` | Replace the field value with `****` |
| `log:"masked:partial"` | Retain only a small visible prefix/suffix |
| `log:"masked:auto"` | Recursively mask sensitive payload fields |
| `log:"masked:headers"` | Mask sensitive HTTP headers |

```go
maskedJSON := goslogx.MaskingLogJSONString("body", `{"password":"secret"}`)
maskedBytes := goslogx.MaskingLogJSONBytes("body", jsonBytes)
maskedHeaders := goslogx.MaskingLogHttpHeaders("headers", headerMap)
```

Masking applies to goslogx log data. It does not automatically sanitize arbitrary OpenTelemetry attributes created by application or third-party instrumentation.

## TRACING AND METRICS

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),

    goslogx.WithResource(goslogx.ResourceConfig{
        ServiceVersion:        "1.5.0",
        DeploymentEnvironment: "production",
        ServiceInstanceID:     "payment-api-01",
        ResourceAttributes: map[string]string{
            "team.name": "payments",
        },
    }),

    goslogx.WithTracing(goslogx.TracingConfig{
        Endpoint:      "collector:4317",
        SamplingRatio: 0.10,
    }),

    goslogx.WithMetrics(goslogx.MetricsConfig{
        Endpoint: "collector:4317",
    }),

    goslogx.WithInternalMetrics(true),
)
```

Check initialization errors before accepting work:

```go
if err := errors.Join(
    logger.TracingError(),
    logger.MetricsError(),
); err != nil {
    return err
}
```

Create custom spans with native OpenTelemetry options:

```go
ctx, span := logger.StartSpan(
    ctx,
    "payment.process",
    trace.WithAttributes(
        attribute.String("payment.type", "debit"),
    ),
)
defer span.End()

logger.InfoContext(
    ctx,
    "",
    "payments",
    goslogx.MESSSAGE_TYPE_EVENT,
    "payment processed",
    nil,
)
```

For native APIs:

```go
tracer := logger.Tracer("payment-service")
meter  := logger.Meter("payment-service")
```

Applications may also inject application-owned providers with `WithTracerProvider` and `WithMeterProvider`.

## GRACEFUL SHUTDOWN

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

_ = server.Shutdown(ctx)
_ = logger.ForceFlush(ctx)
_ = logger.Shutdown(ctx)
```

External providers remain application-owned.

## OPTIONAL INTEGRATIONS

Install only what your application uses:

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fiber
go get github.com/muhammadluth/goslogx/plugins/http/std
go get github.com/muhammadluth/goslogx/plugins/grpc

go get github.com/muhammadluth/goslogx/plugins/database/bun
go get github.com/muhammadluth/goslogx/plugins/database/pgx
go get github.com/muhammadluth/goslogx/plugins/database/gorm

go get github.com/muhammadluth/goslogx/plugins/cache/redis

go get github.com/muhammadluth/goslogx/plugins/messaging/nats
go get github.com/muhammadluth/goslogx/plugins/messaging/kafka/franzgo
go get github.com/muhammadluth/goslogx/plugins/messaging/rabbitmq
```

### HTTP

`net/http`, Fiber, Gin, Echo, Chi, Gorilla Mux, httprouter, go-restful, and fasthttp.

### DATABASE

`database/sql`, pgx, Bun, go-pg, sqlx, GORM, Ent, MongoDB, ClickHouse, Cassandra/ScyllaDB, Elasticsearch, OpenSearch, Neo4j, and DynamoDB.

### CACHE

`go-redis` via `github.com/muhammadluth/goslogx/plugins/cache/redis`.

### MESSAGING

NATS Core/JetStream, Kafka via franz-go/kafka-go/IBM Sarama, and RabbitMQ.

## ARCHITECTURE

```text
                         GOSLOGX CORE
                ┌─────────────────────────┐
                │ LOGGING                 │
                │ TRACING                 │
                │ METRICS                 │
                │ RESOURCE                │
                │ PROPAGATION             │
                │ LIFECYCLE               │
                │ TELEMETRY PROVIDER      │
                └────────────┬────────────┘
                             │
              ┌──────────────┼───────────────┐
              ▼              ▼               ▼
             HTTP         DATABASE        MESSAGING
              │              │               │
          FRAMEWORKS      SQL / ORM      NATS / KAFKA
                                          / RABBITMQ
                             │
                             ▼
                      OPENTELEMETRY SDK
                             │
                            OTLP
                             │
                             ▼
                    OPENTELEMETRY COLLECTOR
```

See [ARCHITECTURE](docs/architecture/README.md).

## BACKEND AGNOSTIC BY DESIGN

```text
GOSLOGX → OPENTELEMETRY → OTLP → COLLECTOR → BACKEND
```

goslogx does not require a vendor-specific observability SDK in application code.

## COMMERCIAL USE

goslogx is distributed under the [MIT LICENSE](LICENSE).

The MIT license permits use, modification, distribution, and inclusion in commercial software subject to its terms and copyright notice requirements.

For production adoption, review version pinning, privacy policy, sampling, retention, dependency compatibility, and operational limits as part of your normal production-readiness process.

## DOCUMENTATION

Start with the [DOCUMENTATION INDEX](docs/README.md).

| Area | Guide |
| --- | --- |
| Observability setup | [OBSERVABILITY](docs/guides/observability.md) |
| Distributed tracing | [TRACING](docs/guides/tracing.md) |
| Metrics | [METRICS](docs/guides/metrics.md) |
| Cardinality and privacy | [CARDINALITY AND PRIVACY](docs/guides/cardinality.md) |
| Production lifecycle | [PRODUCTION](docs/guides/production.md) |
| Architecture | [ARCHITECTURE](docs/architecture/README.md) |
| Plugin ecosystem | [INTEGRATIONS](docs/integrations/README.md) |
| Collector deployment | [OBSERVABILITY PRODUCTION](docs/guides/observability-production.md) |

The public API reference is available on [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).

## EXAMPLES

```text
examples/
├── basic/
├── full-observability/
├── http/
├── database/
├── cache/
└── messaging/
```

The full-observability example demonstrates an end-to-end trace across HTTP, database, Redis, messaging, workers, and outbound HTTP.

## DEVELOPMENT

```bash
make test-all
make check-core
```

Or:

```bash
bash scripts/test-modules.sh
BENCHMARKS=1 bash scripts/test-modules.sh
```

Root `go test ./...` covers the core module only; nested plugins are separate Go modules and must be validated independently.

## LICENSE

goslogx is licensed under the [MIT LICENSE](LICENSE).
