# Architecture

The core module provides structured logging, OpenTelemetry tracing and metrics, shared service resources, propagation, and provider lifecycle management. Optional plugin modules connect those capabilities to application frameworks and clients.

```text
Application
    │
    ├── goslogx core: logs, traces, metrics, resources, lifecycle
    │
    └── optional plugins: Fiber, HTTP, Bun, go-pg, Redis
                  │
                  ▼
           OpenTelemetry SDK
                  │ OTLP/gRPC
                  ▼
       OpenTelemetry Collector
                  │
                  ▼
        Your observability backend
```

## Core and optional dependencies

Import `github.com/muhammadluth/goslogx` for generic observability. Each integration has its own module under `github.com/muhammadluth/goslogx/plugins/`. Installing the core does not pull framework, ORM or Redis dependencies.

Plugins accept `TelemetryProvider`, which supplies a tracer provider, meter provider and text-map propagator. They use explicit providers, preserve application context and leave provider shutdown to the owner. See [plugin authoring](plugins.md#author-a-plugin) to build an integration for another library.

## Resources and ownership

`WithResource` configures service identity for both managed signals. `WithTracing` and `WithMetrics` create providers owned by goslogx; `WithTracerProvider` and `WithMeterProvider` accept providers owned by the application. Managed and external configuration for the same signal are mutually exclusive.

Global registration is opt-in. Integrations work with tracing only, metrics only, or explicit no-op providers. Applications own their HTTP servers/transports, database pools and Redis clients. See [observability](observability.md) for configuration and [production](production.md) for shutdown order.

## Context across signals

Pass the active request context to each operation:

```text
HTTP request span
    └── application span
        ├── context log with obs_trace_id and obs_span_id
        ├── database span
        ├── Redis span
        └── outbound HTTP span
```

HTTP plugins extract incoming trace context and inject outgoing context. Database and Redis operations create child spans from the context passed by the application. Custom tracers and meters use stable instrumentation scopes that identify code, rather than requests or users.

## Export and backend selection

Managed providers export through OTLP/gRPC to a Collector. Configure routing to your chosen backend in the Collector; goslogx does not require a vendor-specific backend SDK. Managed tracing uses batching, and managed metrics use a periodic reader. Applications can supply external providers for other exporter or SDK configurations.

Plugin privacy defaults restrict collected attributes and error details. They do not sanitize arbitrary application-created telemetry. Follow [cardinality and privacy guidance](cardinality.md) when defining business metrics, span attributes and resource values.
