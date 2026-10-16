# GOSLOGX ARCHITECTURE

goslogx separates generic observability capabilities from framework-specific instrumentation.

The core owns logging and generic OpenTelemetry lifecycle concerns. Optional plugins adapt those capabilities to HTTP frameworks, databases, caches, messaging systems, and gRPC without pulling those dependencies into every application.

## ARCHITECTURE OVERVIEW

```text
APPLICATION
    │
    ├── GOSLOGX CORE
    │   ├── structured logging
    │   ├── sensitive-data masking
    │   ├── tracing facade
    │   ├── metrics facade
    │   ├── resource identity
    │   ├── propagation
    │   └── lifecycle
    │
    └── OPTIONAL PLUGINS
        ├── HTTP
        ├── gRPC
        ├── database
        ├── cache
        └── messaging
             │
             ▼
      OPENTELEMETRY SDK
             │
            OTLP
             │
             ▼
   OPENTELEMETRY COLLECTOR
             │
             ▼
    OBSERVABILITY BACKEND
```

## CORE BOUNDARY

Core responsibilities include logging, masking, provider management, resource identity, propagation, correlation, flush/shutdown, and OTLP export.

The core does not import application frameworks, ORMs, cache clients, or messaging clients.

## TELEMETRY PROVIDER CONTRACT

```go
type TelemetryProvider interface {
    TracerProvider() trace.TracerProvider
    MeterProvider() metric.MeterProvider
    TextMapPropagator() propagation.TextMapPropagator
}
```

Plugins consume this small public contract rather than logger internals.

## PROVIDER OWNERSHIP

### GOSLOGX-OWNED PROVIDERS

`WithTracing(...)` and `WithMetrics(...)` create providers owned by goslogx.

### APPLICATION-OWNED PROVIDERS

`WithTracerProvider(...)` and `WithMeterProvider(...)` accept providers owned by the application.

Managed and external configuration for the same signal are mutually exclusive.

## SHARED RESOURCE IDENTITY

`WithResource` defines one service identity shared by managed traces and metrics.

## CONTEXT FLOW

```text
HTTP SERVER SPAN
    │
    └── APPLICATION SPAN
        ├── GOSLOGX LOG
        ├── DATABASE SPAN
        ├── REDIS SPAN
        ├── HTTP CLIENT SPAN
        └── MESSAGE PRODUCER
              ↓
         REMOTE CONSUMER
              ↓
         PROCESS SPAN
```

## INTERNAL TELEMETRY

```text
internal/telemetry/
├── lifecycle/
├── metrics/
├── propagation/
├── resource/
└── tracing/
```

## PLUGIN ARCHITECTURE

```text
plugins/
├── http/
├── grpc/
├── database/
├── cache/
└── messaging/
```

## BACKEND AGNOSTIC DESIGN

```text
GOSLOGX
   ↓
OPENTELEMETRY
   ↓
OTLP
   ↓
COLLECTOR
   ├── GRAFANA
   ├── SIGNOZ
   └── OTHER BACKENDS
```

## PRIVACY AND CARDINALITY

Plugins should default to bounded and privacy-safe telemetry. Avoid secrets, request/response bodies, SQL bind parameters, Redis values, message payloads, and high-cardinality identifiers.

## LIFECYCLE

```text
STOP ACCEPTING WORK
        ↓
FORCE FLUSH
        ↓
SHUT DOWN METRICS
        ↓
SHUT DOWN TRACING
```

## VALIDATION

```bash
make test-all
make check-core
```
