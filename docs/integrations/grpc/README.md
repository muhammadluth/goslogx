# GRPC

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/grpc
```

## UNDERLYING INSTRUMENTATION

The plugin wraps the official `otelgrpc` v0.66.0 stats handlers. `otelgrpc` owns RPC spans, status semantics, message events, propagation, and RPC metrics. goslogx supplies providers, privacy-safe attributes, stable method naming, and signal controls.

## QUICK START

```go
server := grpc.NewServer(grpc.StatsHandler(
    goslogxgrpc.NewServerHandler(logger,
        goslogxgrpc.WithMethodNames("/orders.v1.Orders/Create"))))

conn, err := grpc.NewClient(target,
    grpc.WithTransportCredentials(credentials),
    grpc.WithStatsHandler(goslogxgrpc.NewClientHandler(logger,
        goslogxgrpc.WithMethodNames("/orders.v1.Orders/Create"))))
```

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. W3C context uses the supplied provider propagator. Native RPC span names such as `orders.v1.Orders/Create` and service/method span attributes remain available by default. `WithMethodNames` controls service/method metric dimensions. Status codes remain bounded. Payloads, metadata, endpoints, and raw error descriptions are excluded.

## SUPPORTED OPTIONS

| Capability | goslogx option | Classification | Default |
| --- | --- | --- | --- |
| Tracing | `WithTracing` | Pass through | on |
| Metrics | `WithMetrics` | Pass through | on |
| Propagation | `WithPropagator` | Safe wrapper | provider propagator |
| RPC filter | `WithFilter` | Pass through | all RPCs |
| Public endpoint | `WithPublicEndpoint` | Pass through | off |
| Conditional public endpoint | `WithPublicEndpointFn` | Pass through | unset |
| Message-size events | `WithMessageEvents` | Disabled by default | off |
| Span kind | `WithSpanKind` | Pass through | upstream client/server kind |
| Static span attributes | `WithTraceAttributes`, `WithSpanAttributes` | Safe wrapper | none |
| Static metric attributes | `WithMetricAttributes` | Safe wrapper | none |
| Dynamic metric attributes | `WithMetricAttributesFn` | Safe wrapper | unset |
| Service/method metric labels | `WithMethodNames` | goslogx policy | none |

`WithMethodNames` accepts at most 64 registered full methods of at most 256 bytes each. Metric attributes accept bounded `app.channel`, `service.version`, and `deployment.environment.name` strings. Dynamic attributes pass through the same validation.

Tracer and meter provider overrides are not supported because goslogx owns provider selection. The deprecated upstream interceptor filter and span-options APIs are not applicable to the stats-handler integration.

## PRIVACY DEFAULTS

Message events are disabled. When enabled, they contain only direction, sequence ID, compressed size, and uncompressed size. Message payloads are never recorded. Arbitrary request metadata is not captured.

## PERFORMANCE CONSIDERATIONS

`WithMessageEvents` emits an event for each selected inbound or outbound message, so event volume grows with streaming traffic. Dynamic metric attribute functions run once per RPC and should remain cheap and bounded.

## UPSTREAM CAPABILITY DIFFERENCES

All relevant v0.66.0 stats-handler options are represented. `WithSpanOptions` is intentionally absent because upstream documents it as deprecated and unused by `NewClientHandler` and `NewServerHandler`. Provider overrides remain under goslogx ownership.

## AUTO-INSTRUMENTATION COEXISTENCE

Install one stats handler per client or server. Do not combine this plugin with another gRPC stats handler that creates OpenTelemetry RPC spans or metrics for the same operation.

## TRACING AND METRICS

Client and server stats handlers preserve native method names, status codes, context and
optional message-size events. RPC duration and message-size instruments retain the names and
units of the pinned otelgrpc version. `WithMethodNames` bounds method labels on metrics while
span methods remain available without an allowlist.

## ADVANCED CONFIGURATION, LIFECYCLE AND TROUBLESHOOTING

Use public-endpoint options when remote context should be linked instead of used as a parent.
Close clients and gracefully stop servers before flushing application-owned providers. A nil
provider uses no-op signals. If spans are missing, check stats-handler registration on both
sides and pass the active RPC context to downstream clients.
