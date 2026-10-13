# goslogx

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/graph/badge.svg?token=QD1YFY5MC8)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Go library for structured logging, OpenTelemetry tracing and metrics, and trace/log correlation. Built on [Zap](https://github.com/uber-go/zap), goslogx includes sensitive-field masking and optional integrations for HTTP, Fiber, Bun, go-pg and Redis.

Read the [documentation](docs/README.md) for configuration, integrations and production usage.

## Installation

```bash
go get github.com/muhammadluth/goslogx
```

Core requires Go 1.24 or later. Fiber and Redis plugins require Go 1.26. Each plugin has its own module, so installing core does not pull framework, ORM or Redis dependencies.

## Basic logging

Initialize the logger once during application startup:

```go
package main

import "github.com/muhammadluth/goslogx"

func main() {
    logger := goslogx.New(
        goslogx.WithServiceName("payment-api"),
        goslogx.WithMasking(true),
    )

    logger.Info("", "payments", goslogx.MESSSAGE_TYPE_EVENT,
        "payment received", map[string]any{"result": "accepted"})
}
```

`New` configures a singleton: the first call sets the logger configuration, and subsequent calls return that logger. Package-level functions such as `goslogx.Info` use the same instance.

| Option | Purpose |
| --- | --- |
| `WithServiceName(name)` | Set the application name in logs and managed telemetry resources |
| `WithOutput(writer)` | Choose the log output; defaults to stdout |
| `WithDebug(true)` | Enable debug-level output |
| `WithMasking(true)` | Enable field masking; enabled by default |

Use `Info`, `Debug`, `Warning`, `Error` and `Fatal` for log levels, or their `Context` equivalents to correlate logs with active spans. `Fatal` logs and exits the process. Message types include `MESSSAGE_TYPE_REQUEST`, `MESSSAGE_TYPE_RESPONSE` and `MESSSAGE_TYPE_EVENT`.

## Sensitive-field masking

Use struct tags to describe how sensitive fields should appear in logs:

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
| `log:"masked:partial"` | Retain the first and last two characters |
| `log:"masked:auto"` | Recursively mask payload fields based on sensitive names |
| `log:"masked:headers"` | Mask sensitive HTTP header values |

`HTTPData`, `DBData`, `MQData` and `GenericData` provide standard log payloads with masking tags. You can also mask JSON or headers before logging:

```go
maskedJSON := goslogx.MaskingLogJSONString("body", `{"password":"secret"}`)
maskedBytes := goslogx.MaskingLogJSONBytes("body", jsonBytes)
maskedHeaders := goslogx.MaskingLogHttpHeaders("headers", headerMap)
```

Masking does not sanitize arbitrary OpenTelemetry attributes. See [cardinality and privacy](docs/cardinality.md) when defining telemetry fields.

## Tracing and metrics

Configure managed telemetry and shared service identity during the first `New` call:

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithResource(goslogx.ResourceConfig{
        ServiceVersion:        "1.5.0",
        DeploymentEnvironment: "production",
        ServiceInstanceID:     "payment-api-01",
    }),
    goslogx.WithTracing(goslogx.TracingConfig{Endpoint: "collector:4317"}),
    goslogx.WithMetrics(goslogx.MetricsConfig{Endpoint: "collector:4317"}),
    goslogx.WithInternalMetrics(true),
)
if err := errors.Join(logger.TracingError(), logger.MetricsError()); err != nil {
    return err
}
```

This example uses Go's `errors` package. Managed providers export OTLP/gRPC to a Collector. Configure TLS and authentication for your deployment; choose backends in the Collector. Global OpenTelemetry registration remains opt-in.

Use the active context for application spans and correlated logs:

```go
ctx, span := logger.StartSpan(ctx, "payment.process",
    trace.WithAttributes(attribute.String("payment.type", "debit")),
)
defer span.End()

logger.InfoContext(ctx, "", "payments", goslogx.MESSSAGE_TYPE_EVENT,
    "payment processed", nil)
```

The example uses OpenTelemetry's `trace` and `attribute` packages. Context logs include `obs_trace_id`, `obs_span_id` and `obs_trace_flags`; the existing `trace_id` field remains the application-supplied identifier. `StartTrace` remains available as a simpler API returning a context and end function.

Use `logger.Tracer("payment-service")` and `logger.Meter("payment-service")` for native OpenTelemetry APIs. You can supply application-owned providers through `WithTracerProvider` and `WithMeterProvider`. Managed and external configuration for the same signal are mutually exclusive. Applications own the lifecycle of external providers.

After stopping application work, call `ForceFlush` and `Shutdown` with a bounded context for managed providers. See [observability](docs/observability.md), [tracing](docs/tracing.md), [metrics](docs/metrics.md) and [production](docs/production.md) for details.

## Optional integrations

Install the plugin modules your application needs. The install commands require published plugin versions and a core version containing the telemetry APIs; use the repository's local replacements when working from this checkout.

```bash
go get github.com/muhammadluth/goslogx/plugins/fiber
go get github.com/muhammadluth/goslogx/plugins/http
go get github.com/muhammadluth/goslogx/plugins/bun
go get github.com/muhammadluth/goslogx/plugins/pg
go get github.com/muhammadluth/goslogx/plugins/redis
```

| Integration | Guide |
| --- | --- |
| Fiber v3 HTTP server | [Fiber](docs/integrations/fiber.md) |
| net/http server and client | [HTTP](docs/integrations/http.md) |
| Bun ORM | [Bun](docs/integrations/bun.md) |
| go-pg v10 compatibility | [go-pg](docs/integrations/pg.md) |
| go-redis v9 clients | [Redis](docs/integrations/redis.md) |

Plugins accept `goslogx.TelemetryProvider`, use explicit providers and preserve application context. Tracing-only, metrics-only and no-op configurations work without requiring global providers.

The [full observability example](examples/full_observability/README.md) connects Fiber, Bun, Redis and outbound HTTP in one trace. See [plugins](docs/plugins.md) for configuration and the plugin authoring contract.

## Documentation

Start at the [documentation index](docs/README.md).

- [Observability configuration and ownership](docs/observability.md)
- [Tracing and log correlation](docs/tracing.md)
- [Managed and custom metrics](docs/metrics.md)
- [Architecture](docs/architecture.md)
- [Plugins and authoring](docs/plugins.md)
- [Cardinality and privacy](docs/cardinality.md)
- [Production and graceful shutdown](docs/production.md)
- [Collector and infrastructure](docs/observability-production.md)

The public API reference is available on [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).

## Development

Run checks for core, every plugin module and the combined example:

```bash
bash scripts/test-modules.sh
```

To include benchmarks:

```bash
BENCHMARKS=1 bash scripts/test-modules.sh
```

Root `go test ./...` excludes nested modules. The validation script runs dependency checks, vet, unit tests, race tests and coverage for each module. Tests use in-process OTLP receivers, SQL mocks and miniredis without an external Collector or database.

## License

[MIT](LICENSE).
