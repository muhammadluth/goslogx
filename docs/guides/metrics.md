# Metrics

`WithMetrics(MetricsConfig{Endpoint: "collector:4317"})` creates an OTLP/gRPC exporter, PeriodicReader and logger-owned MeterProvider. The reader exports every minute by default; set `ExportInterval` and `ExportTimeout` to bound exports. TLS, mTLS, headers, gzip compression and opt-in global registration follow tracing configuration. Use TLS in production. `WithMetrics` clones header maps and TLS settings.

Use `WithMeterProvider(mp)` for application-owned metrics. goslogx leaves that provider's registration, resource, flush and shutdown to you. It supports metrics without tracing. With neither mode enabled, `MeterProvider()` returns an explicit no-op provider.

```go
meter := logger.Meter("payment-service")
counter, err := meter.Int64Counter("payment.transactions")
if err != nil { return err }
counter.Add(ctx, 1, metric.WithAttributes(
    attribute.String("payment.type", "debit"),
    attribute.String("payment.result", "success"),
))
```

Package-level `Meter` uses the global logger. Empty names use the goslogx scope. OTel APIs remain available for business instruments, views, readers and exemplars. Applications own their business dimension policy; see [cardinality](cardinality.md).

Enable `WithInternalMetrics(true)` to retain `goslogx.log.records`, `goslogx.telemetry.errors`, force-flush duration/errors, and shutdown duration/errors. Internal health metrics use fixed severity/operation/result labels and remain separate from plugin/application metrics. Lifecycle observations occur after each call; the final shutdown observation cannot export through a provider that has closed.

HTTP plugins use upstream instruments, with no duplicate goslogx HTTP metrics. DB plugins record `db.client.operation.duration` in seconds (histogram count supplies operation/error counts) and `db.client.connection.count` by idle/used state. Redis uses upstream redisotel duration and pool instruments; its pinned upstream package retains legacy metric names. See each integration guide for details.
