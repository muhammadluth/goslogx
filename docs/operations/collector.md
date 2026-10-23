# OPENTELEMETRY COLLECTOR

Managed tracing and metrics export over OTLP/gRPC to the configured endpoint, commonly port `4317`. Point both signals at a Collector or Alloy instance that has OTLP receivers and separate traces and metrics pipelines.

```text
goslogx application
    -> OTLP/gRPC :4317
    -> Collector or Alloy
       -> traces backend
       -> metrics backend
```

The runnable configuration under [`deployments/otel-collector`](../../deployments/otel-collector/README.md) includes local Docker Compose, Kubernetes, TLS, retry, persistent queue, and host-metrics variants.

Use `Insecure: true` only for local plaintext endpoints. Production endpoints should use `TLSConfig` and authentication headers supplied by a secret manager. Monitor Collector port `8888` for exporter failures and queue pressure and port `13133` for health.
