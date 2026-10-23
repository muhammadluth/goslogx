# PLUGIN MODEL

Plugins adapt a framework or client to the public goslogx telemetry provider. When mature upstream OpenTelemetry instrumentation exists, it remains responsible for telemetry semantics:

```text
application -> goslogx plugin -> upstream instrumentation -> OpenTelemetry
```

Examples include `otelhttp` for `net/http`, `otelgrpc` for gRPC, and `redisotel` for go-redis. Integrations without a maintained upstream package use stable hooks such as Bun `QueryHook` or pgx tracer interfaces. Wrappers are used only when hooks are insufficient.

Privacy-sensitive and costly features such as statement, caller, body, or payload capture are disabled by default and exposed only where the plugin implements a safe opt-in. Do not enable a goslogx plugin and automatic instrumentation for the same library unless the integration guide explicitly permits it; duplicate layers can create duplicate spans, metrics, incorrect parentage, and extra overhead.
