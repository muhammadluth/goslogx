# Architecture

The core module provides structured logging, OpenTelemetry tracing and metrics, shared service resources, propagation, and provider lifecycle management. Optional plugin modules connect those capabilities to application frameworks and clients.

```text
Application
    │
    ├── goslogx core: logs, traces, metrics, resources, lifecycle
    │
    └── optional plugins: HTTP frameworks, SQL/ORM/NoSQL, Redis, messaging
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

Import `github.com/muhammadluth/goslogx` for generic observability. Each integration has its own module under `github.com/muhammadluth/goslogx/plugins/`. Installing the core does not pull framework, database or messaging dependencies.

Plugins accept `TelemetryProvider`, which supplies a tracer provider, meter provider and text-map propagator. They use explicit providers, preserve application context and leave provider shutdown to the owner. See [plugin authoring](../integrations/README.md#author-a-plugin) to build an integration for another library.

## Resources and ownership

`WithResource` configures service identity for both managed signals. `WithTracing` and `WithMetrics` create providers owned by goslogx; `WithTracerProvider` and `WithMeterProvider` accept providers owned by the application. Managed and external configuration for the same signal are mutually exclusive.

Global registration is opt-in. Integrations work with tracing only, metrics only, or explicit no-op providers. Applications own their HTTP servers/transports, database pools and Redis clients. See [observability](../guides/observability.md) for configuration and [production](../guides/production.md) for shutdown order.

## Context across signals

Pass the active request context to each operation:

```text
HTTP request span
    └── application span
        ├── context log with obs_trace_id and obs_span_id
        ├── database span
        ├── Redis span
        ├── outbound HTTP span
        └── message producer
            └── remote receipt
                └── message process
                    └── worker database span
```

HTTP plugins extract incoming trace context and inject outgoing context. Database and Redis operations create child spans from the context passed by the application. Custom tracers and meters use stable instrumentation scopes that identify code, rather than requests or users.

## Export and backend selection

Managed providers export through OTLP/gRPC to a Collector. Configure routing to your chosen backend in the Collector; goslogx does not require a vendor-specific backend SDK. Managed tracing uses batching, and managed metrics use a periodic reader. Applications can supply external providers for other exporter or SDK configurations.

Plugin privacy defaults restrict collected attributes and error details. They do not sanitize arbitrary application-created telemetry. Follow [cardinality and privacy guidance](../guides/cardinality.md) when defining business metrics, span attributes and resource values.

Messaging adapters inject/extract headers using the provider propagator. See [integration policy](integration-policy.md) for context continuity, bounded destinations and choosing an instrumentation layer. Bun and go-pg use their canonical `plugins/database` paths.

## HTTP adapters

The HTTP namespace contains std, Fiber, Gin, Echo, Chi, Gorilla Mux, httprouter, go-restful and fasthttp modules. Native framework adapters configure official OTel middleware where available. Chi and httprouter share otelhttp; fasthttp uses native API instrumentation. Shared HTTP provider/privacy policy keeps framework dependencies out of core. See [HTTP integrations](../integrations/http/README.md) for routes, errors and supported options.

## Core telemetry implementation

The root package is the public facade. `tracing.go`, `metrics.go`, `resource.go`, `propagation.go` and `lifecycle.go` expose configuration and operations without constructing SDK exporters. `telemetry.go` defines the public tracing/metrics capability interfaces. Context logging and span correlation remain in the logging layer.

`internal/telemetry/runtime.go` composes managed and external providers. Its resource builder merges shared identity with legacy tracing identity once, then supplies that same resource to both owned providers. Tracing owns exporter, batching, sampling and transport validation; metrics owns exporter, reader and internal metric instruments. The lifecycle coordinator owns synchronization, flushing, shutdown, global registration/restoration and throttled SDK error reporting. Applications and plugins do not import these packages.

Optional plugins use public `goslogx` capabilities. `internal/integration` contains shared integration policies and testkits; plugins consume these private helpers alongside the public core contract. HTTP and gRPC contrib instrumentation live in their nested modules; the core's gRPC transport dependency serves OTLP exporters. Collector templates live under `deployments/otel-collector`. Run `make test-all` for every module or `make check-core` to verify dependency boundaries.
