# ECHO V4

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/echo
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

```go
e := echo.New()
e.Use(goslogxecho.Middleware(logger,
    goslogxecho.WithServerName("orders-api")))
e.GET("/orders/:id", func(c echo.Context) error {
    ctx := c.Request().Context()
    return loadOrder(ctx, c.Param("id"))
})
```

The plugin wraps official `otelecho` v0.71.0 and uses `c.Path()` templates. `otelecho` owns spans, metrics, routing, and error semantics; goslogx owns providers and privacy filtering.

## DEFAULT BEHAVIOR AND OPTIONS

Tracing and metrics are enabled. The active context is in `c.Request().Context()`. Supported options include `WithSkipper`, request and Echo metric attribute callbacks, `WithOnError`, request filters, static trace/metric attributes, signal controls, server name, and propagator. Dynamic metric callbacks accept only bounded approved application/environment/version keys. Provider overrides are not exposed.

Headers, bodies, query values, and raw error descriptions are excluded. Callback and skipper functions run per request and should remain cheap.

Your configured `HTTPErrorHandler` receives the original handler error and controls the response. The upstream middleware dispatches it before recording status; this adapter consumes that handled error to prevent Echo dispatching it again. Outer middleware therefore sees a handled error as nil. Filtered and skipped requests retain Echo's normal error flow; the skipper runs once. A custom `WithOnError` callback must preserve the intended Echo error-handler behavior.

## UPSTREAM CAPABILITY DIFFERENCES AND COEXISTENCE

All public `otelecho` v0.71.0 options are represented except tracer and meter provider overrides, which are not applicable. Echo's native span naming is used because upstream exposes no formatter option. Do not wrap this adapter in another `otelecho` middleware. Configure recovery inside telemetry when its final response status must be observed.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrate downstream correlation.

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
