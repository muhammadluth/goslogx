# PLUGIN MODEL

Plugins adapt a framework or client to the public goslogx telemetry provider. When mature upstream OpenTelemetry instrumentation exists, it remains responsible for telemetry semantics:

```text
application -> goslogx plugin -> upstream instrumentation -> OpenTelemetry
```

Examples include `otelhttp` for `net/http`, `otelgrpc` for gRPC, `redisotel` for go-redis, `bunotel` for Bun, `otelpgx` for pgx and `kotel` for native franz-go hooks. Integrations without a maintained upstream package use stable hooks such as Cassandra query observers. Wrappers add goslogx provider selection, stable scope, compatibility metrics, and privacy enforcement where upstream defaults expose sensitive data.

Privacy-sensitive and costly features such as statement, caller, body, or payload capture are disabled by default and exposed only where the plugin implements a safe opt-in. Do not enable a goslogx plugin and automatic instrumentation for the same library unless the integration guide explicitly permits it; duplicate layers can create duplicate spans, metrics, incorrect parentage, and extra overhead.
