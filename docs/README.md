# GOSLOGX DOCUMENTATION

This documentation explains how to install, configure, integrate, operate, and troubleshoot goslogx in Go applications.

## GETTING STARTED

1. [Install core and optional plugins](getting-started/installation.md).
2. [Create a logger and emit telemetry](getting-started/quick-start.md).
3. [Configure resources, exporters, and ownership](getting-started/configuration.md).
4. Add the required [HTTP, gRPC, database, cache, or messaging integration](integrations/README.md).
5. Configure an [OpenTelemetry Collector](operations/collector.md).
6. Implement [production shutdown](guides/lifecycle.md).

## CORE GUIDES

| Topic | Guide |
| --- | --- |
| Structured logging | [LOGGING](guides/logging.md) |
| Sensitive-data masking | [MASKING](guides/masking.md) |
| Custom spans | [TRACING](guides/tracing.md) |
| Custom and internal metrics | [METRICS](guides/metrics.md) |
| Log/trace correlation | [CORRELATION](guides/correlation.md) |
| Context propagation | [PROPAGATION](guides/propagation.md) |
| Resource identity | [RESOURCE](guides/resource.md) |
| Cardinality | [CARDINALITY](guides/cardinality.md) |
| Provider shutdown | [LIFECYCLE](guides/lifecycle.md) |
| Production configuration | [PRODUCTION](guides/production.md) |

## INTEGRATIONS

The [integration index](integrations/README.md) lists canonical module paths and links to HTTP, gRPC, database, Redis, NATS, Kafka, and RabbitMQ guides.

## OPERATIONS

Use the [Collector](operations/collector.md), [Grafana](operations/grafana.md), [production](operations/production.md), and [troubleshooting](operations/troubleshooting.md) guides to run goslogx telemetry outside local development.

## ARCHITECTURE

The [architecture overview](architecture/overview.md) explains provider ownership, plugin boundaries, and telemetry flow at the level needed to integrate or extend the library.

The public Go API is available on [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).
