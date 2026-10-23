# CONFIGURATION

goslogx uses functional options. Configure logging, resource identity, telemetry providers, and privacy during startup.

| Option | Purpose | Ownership |
| --- | --- | --- |
| `WithServiceName` | Sets `application_name` in logs and `service.name` in managed telemetry | Application configuration |
| `WithOutput` | Selects the log writer | Application |
| `WithDebug` | Enables debug logging | goslogx logger |
| `WithMasking` | Enables automatic log-data masking | goslogx logger |
| `WithResource` | Sets shared OpenTelemetry resource identity | goslogx managed providers |
| `WithTracing` | Creates a managed OTLP tracer provider | goslogx |
| `WithMetrics` | Creates a managed OTLP meter provider | goslogx |
| `WithTracerProvider` | Injects an existing tracer provider | Application |
| `WithMeterProvider` | Injects an existing meter provider | Application |
| `WithInternalMetrics` | Emits low-cardinality goslogx metrics through the configured meter provider | Provider owner controls lifecycle |

Managed and external providers are mutually exclusive for the same signal. Configuration errors are retained by the logger and returned by `TracingError()` or `MetricsError()`.

For production OTLP, configure TLS through `TLSConfig`, optional request `Headers`, `Compression: "gzip"`, and exporter timeouts. Use `Insecure: true` only on a trusted development network. See [provider ownership](../architecture/provider-ownership.md) and [production operations](../operations/production.md).
