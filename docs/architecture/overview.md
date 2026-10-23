# ARCHITECTURE OVERVIEW

goslogx core provides structured logging, tracing and metrics facades, propagation, resource identity, and lifecycle. Framework and client integrations are separate plugin modules.

```text
application
    -> goslogx logging and telemetry facade
    -> optional plugin
    -> OpenTelemetry API/SDK
    -> OTLP Collector
    -> logs, metrics, and trace backends
```

Core does not import Fiber, Gin, database drivers, Redis, or message brokers. This keeps application dependencies explicit. Plugins receive the public `TelemetryProvider` contract and use its tracer provider, meter provider, and propagator.

Continue with [provider ownership](provider-ownership.md), the [plugin model](plugin-model.md), and [telemetry flow](telemetry-flow.md).
