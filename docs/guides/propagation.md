# CONTEXT PROPAGATION

Propagation carries a parent trace across process and transport boundaries. goslogx defaults to W3C Trace Context and exposes `logger.TextMapPropagator()` to plugins.

For manual HTTP code, use `InjectHTTPContext` before sending a request and `ExtractHTTPContext` when accepting one. Prefer the HTTP plugins because they combine propagation with server/client telemetry.

HTTP and gRPC plugins extract and inject context through protocol metadata. NATS, Kafka, and RabbitMQ plugins use message headers. Consumer code must pass the extracted context into processing and context-aware logging so child spans keep the producer trace.

Use plugin-specific `WithPropagator` options where exposed. Configure the same propagation format on every participating service. See the [integration index](../integrations/README.md) and [correlation](correlation.md).
