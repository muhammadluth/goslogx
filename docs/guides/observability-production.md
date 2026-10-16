# Production observability checklist

This guide covers Collector deployment, application-owned providers, and
infrastructure signals. See [production](production.md) for managed provider
lifecycle and [metrics](metrics.md) for metrics configuration.

## Architecture boundary

```text
goslogx
├── accepts a TracerProvider
├── accepts a MeterProvider
├── correlates logs with active traces
├── optionally records bounded internal logger metrics
└── does not know HTTP, DB, or NATS objects

application
├── HTTP RED metrics
├── bounded business-event metrics
├── DB pool observer
└── NATS consumer metrics

infrastructure
├── CPU, memory, and network
├── container metrics
└── Collector health and queue metrics
```

## Ownership matrix

| Signal/provider | Configured by | ForceFlush and Shutdown owner | `goslogx` responsibility |
| --- | --- | --- | --- |
| Managed tracing | `WithTracing` | `goslogx` | creates the SDK, exports spans, and may register globally only with `RegisterGlobal` |
| External tracing | `WithTracerProvider` | application | uses it for tracing APIs, correlation and plugin access |
| Managed metrics | `WithMetrics` | `goslogx` | creates an OTLP/gRPC PeriodicReader SDK with shared resource identity |
| External metrics | `WithMeterProvider` | application | uses it for application accessors and optional internal metrics only |
| HTTP, SQL DB, NATS clients | application | application | none |
| Host/container runtime | platform/Collector | platform/Collector | none |

`WithTracing` and `WithTracerProvider` are mutually exclusive. `WithMeterProvider`
never changes the OpenTelemetry global provider; omitted or nil providers are
safe no-op providers.

## External MeterProvider setup

Create the metrics SDK in the application with the same service resource used
by the deployment, then inject it into `goslogx`:

```go
meterProvider := sdkmetric.NewMeterProvider(
    sdkmetric.WithResource(resource.NewWithAttributes(
        semconv.SchemaURL,
        semconv.ServiceName("payment-api"),
    )),
    sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
)

logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithMeterProvider(meterProvider),
    goslogx.WithInternalMetrics(true), // optional
)
```

Use `logger.MeterProvider()` to create application instruments. The provider is
still owned by the application; `goslogx.ForceFlush` and `goslogx.Shutdown` do
not flush or close it.

## Application metric references

| Need | Reference | Rules |
| --- | --- | --- |
| HTTP RED | [`examples/basic/metrics`](../../examples/basic/metrics) | use the HTTP plugin for matched-route request telemetry, never a raw URL |
| Business events | [`examples/basic/metrics`](../../examples/basic/metrics) | use fixed event names such as `orders.created`; never add a payment ID |
| DB pool | [SQL plugin](../integrations/database/sql.md) | observe `sql.DBStats`; keep the `*sql.DB` in the application |
| NATS consumer lag | [NATS plugin](../integrations/messaging/nats.md) | record pending, ack-pending, redelivery, and lag with fixed destination/consumer names |

Always record with the request or consumer context. A sampled active span can
then be associated with a metric exemplar by the metrics SDK.

## Runtime and infrastructure sources

| Metric | Source |
| --- | --- |
| Go heap, GC, goroutines | application Go runtime instrumentation |
| Container CPU/memory/network | cAdvisor or kubelet |
| Host CPU/memory/network | node exporter or Collector `hostmetrics` |
| Collector process, accepted/refused data, queue | Collector internal telemetry on port `8888` |
| Log records and bytes | Loki/Alloy pipeline metrics |

`goslogx` does not collect these infrastructure signals. Deploy
`hostmetrics` once per node, usually as a DaemonSet, rather than at every
trace-gateway replica.

## Collector and metrics backend

[`collector-hostmetrics.yaml`](../../deployments/otel-collector/collector-hostmetrics.yaml) provides
separate trace and metric pipelines. The metric pipeline accepts application
OTLP metrics and host metrics, then sends them to a metrics backend. Tempo is
trace-only and must not be the metric exporter target.

For a Prometheus pull deployment, replace the metrics exporter with a
Prometheus exporter and scrape it:

```yaml
exporters:
  prometheus/application:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    metrics:
      exporters: [prometheus/application]
```

```yaml
scrape_configs:
  - job_name: otel-application-metrics
    static_configs:
      - targets: [otel-collector.observability.svc:9464]
```

For Grafana Mimir or another Prometheus Remote Write backend, use the Contrib
`prometheusremotewrite` exporter instead:

```yaml
exporters:
  prometheusremotewrite/mimir:
    endpoint: https://mimir.example.com/api/v1/push
    headers:
      Authorization: ${env:MIMIR_AUTHORIZATION}

service:
  pipelines:
    metrics:
      exporters: [prometheusremotewrite/mimir]
```

Store `MIMIR_AUTHORIZATION` in a secret manager or workload secret; do not
commit it. Ensure the deployed Collector Contrib distribution contains the
selected exporter.

## Cardinality and privacy rules

Only bounded, predefined values may be metric attributes. For `goslogx`
internal metrics the complete allowlist is:

```text
severity = DEBUG | INFO | WARNING | ERROR | CRITICAL
operation = error_handler | force_flush | shutdown
result = success | error
```

Never use trace/span/request/user IDs, error text, raw URLs, message text,
modules, payloads, or generated NATS subjects as metric attributes. Use route
templates and fixed business event names instead.

## Graceful shutdown

Stop accepting application traffic first. The following order keeps metrics
SDK ownership with the application and tracing SDK ownership with `goslogx`:

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

_ = server.Shutdown(shutdownCtx)

// MeterProvider is owned by the application.
_ = meterProvider.ForceFlush(shutdownCtx)
_ = meterProvider.Shutdown(shutdownCtx)

// Managed tracing is owned by goslogx.
_ = logger.ForceFlush(shutdownCtx)
_ = logger.Shutdown(shutdownCtx)
```

When internal logger metrics are enabled, the last `logger.ForceFlush` and
`logger.Shutdown` lifecycle observations occur after the MeterProvider closes
in this order and therefore cannot be exported. This does not affect tracing
flush/shutdown. If those final internal lifecycle observations are required,
flush/shut down the logger before the MeterProvider, then flush the provider.

## Release checklist

- [ ] Service resource, Collector endpoint, TLS, and authentication are configured.
- [ ] The application owns and shuts down external tracer/meter providers.
- [ ] HTTP uses matched route patterns; business events use fixed names.
- [ ] DB and NATS references use low-cardinality attributes only.
- [ ] Runtime, container, host, Collector, and log-volume metrics have the correct source.
- [ ] Metrics go to Prometheus, Mimir, or another metrics backend; traces go to Tempo.
- [ ] Collector queue, failed exports, CPU, memory, and disk are alerted on.
- [ ] Credentials are secret-managed and absent from logs/config commits.
- [ ] Cardinality tests and benchmarks meet the agreed budget.

Run the final gates before release:

```bash
go test ./...
go test -race ./...
go vet ./...
go test -bench='Metric|Context' -benchmem ./...
```
