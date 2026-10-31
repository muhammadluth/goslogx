# NET/HTTP

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/std
```

## UNDERLYING INSTRUMENTATION

Import `github.com/muhammadluth/goslogx/plugins/http/std` as `goslogxhttp`. Wrap an application handler or existing RoundTripper:

## QUICK START

```go
handler := goslogxhttp.NewHandler(logger, handler, "GET payments",
    goslogxhttp.WithRoute("/payments/{id}"),
)
client := &http.Client{
    Timeout: 3*time.Second,
    Transport: goslogxhttp.NewTransport(logger, customTransport),
}
```

Official `otelhttp` v0.66.0 produces server/client spans, W3C propagation, and request-duration/body-size histograms. The stable instrumentation scope is `github.com/muhammadluth/goslogx/plugins/http/std`; upstream instruments remain intact.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Full URLs, query strings, user agents, headers, request-derived client/server addresses, body contents, and raw error events are excluded. Route templates are recorded when supplied statically or discovered from Go 1.22+ ServeMux patterns.

## SUPPORTED OPTIONS

| Capability | goslogx option | Classification | Default |
| --- | --- | --- | --- |
| Signal controls | `WithTracing`, `WithMetrics` | Pass through | both on |
| Request filtering | `WithRequestFilter` | Pass through | all requests |
| Propagation | `WithPropagator` | Safe wrapper | provider propagator |
| Route template | `WithRoute` | Safe wrapper | unset |
| Span naming | `WithSpanNameFormatter` | Safe wrapper | upstream/default route naming |
| Span start configuration | `WithSpanStartOptions` | Safe wrapper | none |
| Public endpoint | `WithPublicEndpoint`, `WithPublicEndpointFn` | Pass through | off |
| Body-size events | `WithMessageEvents` | Disabled by default | off |
| Static span attributes | `WithTraceAttributes` | Safe wrapper | none |
| Static metric attributes | `WithMetricAttributes` | Safe wrapper | none |
| Dynamic metric attributes | `WithMetricAttributesFn` | Safe wrapper | unset |
| Client trace callbacks | `WithClientTrace` | Pass through | unset |
| Server identity | `WithServerName` | Safe wrapper | unset |

Provider overrides are not supported because goslogx owns provider selection. `WithMetricAttributesFn` exists for upstream parity even though upstream has deprecated it in favor of Labeler; goslogx validates and isolates its returned attributes from request-controlled Labeler attributes.

The wrapper preserves a supplied transport's timeout/proxy/TLS/pooling configuration. Nil selects `http.DefaultTransport`. Close response bodies to finish client spans. It clones requests and disables upstream GetBody replay on the private clone to prevent raw replay errors from reaching the global error handler; it restores GetBody before calling the custom base transport, so application replay behavior remains available.

## PRIVACY DEFAULTS

The attribute boundary drops full URLs, query strings, user agents, headers, request-derived client/server addresses, bodies, and raw error text. Dynamic metric attributes accept only bounded environment/version and approved `app.*` keys. Application request data and returned errors remain available to handlers and callers.

## PERFORMANCE CONSIDERATIONS

Body-size message events emit one event for each selected read or write and remain disabled by default. Dynamic attribute and client-trace callbacks run on every request. Keep them cheap.

## UPSTREAM CAPABILITY DIFFERENCES

All public `otelhttp` v0.66.0 configuration capabilities are represented except provider overrides, which are not applicable. `otelhttp` exposes no server active-request instrument in this version, and goslogx adds no duplicate metric.

## AUTO-INSTRUMENTATION COEXISTENCE

Install a single HTTP instrumentation layer. Do not wrap a handler or transport with this plugin and another `otelhttp` layer for the same operation.

## HTTP NAMESPACE AND SERVEMUX

The canonical import path is `github.com/muhammadluth/goslogx/plugins/http/std`. Go 1.22+ ServeMux patterns are read after dispatch: `GET /users/{id}` is recorded as that template rather than the concrete path. `InstrumentClient` returns a configured copy of your client while wrapping its existing transport.

See [HTTP integrations](README.md) for router-specific choices, common options and automatic-instrumentation coexistence, and [HTTP examples](../../../examples/http/README.md) for downstream tests.

## TRACING AND METRICS

Server instrumentation records SERVER spans with method, registered route and response status.
Incoming context uses the configured propagator. Request-duration histograms use seconds; route
labels use templates. Safe upstream events, links and error types are retained. Raw exception and
panic messages are redacted while the original failure remains available to the application.

For signal modes, bounded attributes and provider ownership, see the
[HTTP integration contract](README.md#options-and-context). Native middleware determines which
body-size or active-request instruments are available; go-restful and fasthttp supply the shared
duration and active-request instruments described in their guides.

## LIFECYCLE AND TROUBLESHOOTING

Install instrumentation before accepting requests. Pass the handler's active context downstream,
finish requests, then flush and shut down application-owned providers. Use one instrumentation
layer per request. If route labels are absent, check registration order and the framework's
unmatched-route behavior. Callback filters suppress telemetry without suppressing the handler.
