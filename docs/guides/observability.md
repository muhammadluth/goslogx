# Observability

Use goslogx for structured logs, OTel traces and metrics. Send OTLP/gRPC to a Collector; choose backends in the Collector configuration. Core imports no Fiber, ORM or Redis packages.

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithResource(goslogx.ResourceConfig{
        ServiceVersion: "1.5.0",
        DeploymentEnvironment: "production",
        ServiceInstanceID: "payment-api-01",
        ResourceAttributes: map[string]string{"team.name": "payments"},
    }),
    goslogx.WithTracing(goslogx.TracingConfig{Endpoint: "collector:4317"}),
    goslogx.WithMetrics(goslogx.MetricsConfig{Endpoint: "collector:4317"}),
    goslogx.WithInternalMetrics(true),
)
if err := errors.Join(logger.TracingError(), logger.MetricsError()); err != nil {
    return err
}
```

`New` retains its existing singleton behavior: only the first call configures the global logger. Configure it once before starting work. Package functions use that logger. Neither provider registers globally unless you set `RegisterGlobal: true`.

Use `WithResource` for both signals. Nonempty shared fields override legacy `TracingConfig` identity fields; shared map entries override legacy map entries. Existing tracing resource fields remain supported. Managed telemetry requires a service name, version, environment and instance ID. Both managed providers use the same resource constructor and schema. External providers retain the resources supplied by the application.

`WithTracing` and `WithTracerProvider` are mutually exclusive. `WithMetrics` and `WithMeterProvider` are mutually exclusive. These option functions enable their managed signal; omit the option to disable it. Logging remains available after initialization errors, which you must check. Disabled providers create no exporters or background readers.

See [tracing](tracing.md), [metrics](metrics.md), [plugins](../integrations/README.md), [production](production.md), and [architecture](../architecture/README.md).
