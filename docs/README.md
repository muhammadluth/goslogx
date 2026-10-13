# goslogx documentation

Use goslogx for structured logging, OpenTelemetry traces and metrics, and optional framework integrations.

## Getting started

Start with [basic logging and installation](../README.md). Add telemetry with the [observability guide](observability.md), then choose the integrations your application needs.

| Guide | Contents |
| --- | --- |
| [Observability](observability.md) | Configuration, service resources and provider ownership |
| [Tracing](tracing.md) | Custom tracers, spans, attributes and log correlation |
| [Metrics](metrics.md) | Managed metrics, custom instruments and internal logger metrics |
| [Architecture](architecture.md) | Core and plugin boundaries, context flow and backend integration |
| [Plugins](plugins.md) | Installation, supported integrations and plugin authoring |
| [Cardinality and privacy](cardinality.md) | Safe attributes and sensitive-data handling |
| [Production](production.md) | Initialization, export configuration and graceful shutdown |
| [Collector and infrastructure](observability-production.md) | Deployment, external providers and infrastructure metric sources |

## Integrations

- [Fiber v3](integrations/fiber.md)
- [net/http server and client](integrations/http.md)
- [Bun](integrations/bun.md)
- [go-pg v10](integrations/pg.md)
- [go-redis v9](integrations/redis.md)

The [full observability example](../examples/full_observability/README.md) connects Fiber, Bun, Redis and outbound HTTP in one trace.
