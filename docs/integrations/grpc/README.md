# gRPC

```bash
go get github.com/muhammadluth/goslogx/plugins/grpc
```

Use the official OpenTelemetry stats handlers through the plugin. This optional module requires Go 1.24 and leaves server, connection, credentials and interceptor ownership with the application.

```go
server := grpc.NewServer(grpc.StatsHandler(
    goslogxgrpc.NewServerHandler(logger,
        goslogxgrpc.WithMethodNames("/orders.v1.Orders/Create"))))

conn, err := grpc.NewClient(target,
    grpc.WithTransportCredentials(credentials),
    grpc.WithStatsHandler(goslogxgrpc.NewClientHandler(logger,
        goslogxgrpc.WithMethodNames("/orders.v1.Orders/Create"))))
```

The handlers supply explicit tracer/meter providers and the provider's propagator to `otelgrpc`. They create client/server spans and RPC metrics, propagate W3C context through gRPC metadata and retain native status/error behavior. Unary and streaming events follow the native stats-handler lifecycle. Pass the active context to calls, and use the server handler's context for `InfoContext` and downstream work; `obs_trace_id` and `obs_span_id` correlate logs.

Default span names are `gRPC`; raw service/method metadata is omitted. `WithMethodNames` opts up to 64 registered full methods (at most 256 bytes each) into stable span names and metric dimensions. Do not derive this list from requests. Status codes remain bounded. Payloads, metadata, endpoints and raw error messages/events are excluded. Native errors and responses remain available to callers.

`WithFilter` accepts `*stats.RPCTagInfo`; false bypasses telemetry. `WithTracing(false)` and `WithMetrics(false)` independently disable signals. `WithPropagator` overrides propagation; configure a TraceContext/Baggage composite explicitly if needed. `WithPublicEndpoint` applies to server spans, starting a new root with a remote link. Explicit trace attributes stay span-only. Static metric attributes accept `app.channel`, `service.version` and `deployment.environment.name` string values up to 128 bytes.

Install one stats instrumentation layer. Choose this plugin or automatic/other gRPC instrumentation to avoid duplicate spans and metrics; no external agent detection is attempted. Providers remain application-owned. Normal tests use an in-memory gRPC transport and in-memory OTel providers, without a Collector.
