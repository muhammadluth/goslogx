# GIN

## UNDERLYING INSTRUMENTATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/gin
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

```go
router := gin.New()
router.Use(goslogxgin.Middleware(logger,
    goslogxgin.WithServiceName("orders-api")))
router.GET("/orders/:id", func(c *gin.Context) {
    ctx := c.Request.Context()
    logger.InfoContext(ctx, "", "orders", goslogx.MESSSAGE_TYPE_REQUEST,
        "order requested", nil)
    c.Status(http.StatusOK)
})
```

The plugin wraps official `otelgin` v0.72.0 with explicit tracer, meter, and propagator ownership. `otelgin` owns HTTP spans, metrics, routing, and error semantics. The matched `FullPath()` supplies the route; unmatched requests use a method-only span name and omit the route.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Request and Gin filters allow all requests. Handler `c.Errors` and 5xx responses mark spans as errors; ordinary 4xx responses do not. Headers, bodies, query values, and raw error descriptions are excluded.

## SUPPORTED OPTIONS

| Upstream capability | goslogx option | Classification |
| --- | --- | --- |
| HTTP request filters | `WithFilter`, `WithRequestFilter` | Pass through |
| Gin context filters | `WithGinFilter` | Pass through |
| Span start options | `WithSpanStartOptions` | Safe wrapper |
| Span naming | `WithSpanNameFormatter` | Safe wrapper |
| Request metric callback | `WithMetricAttributeFn`, `WithMetricAttributesFn` | Safe wrapper |
| Gin metric callback | `WithGinMetricAttributeFn`, `WithGinMetricAttributesFn` | Safe wrapper |
| Providers/propagation | provider plus `WithPropagator` | goslogx-owned |
| Signal controls | `WithTracing`, `WithMetrics` | Pass through |
| Static attributes/server name | `WithTraceAttributes`, `WithMetricAttributes`, `WithServerName` | Safe wrapper |

Provider override options are not exposed. Dynamic metric callbacks accept only bounded approved `app.*`, environment, and version keys.

## PRIVACY AND PERFORMANCE

Callbacks run for every request, so keep them cheap and never derive metric values from user IDs, paths, or other unbounded input. Span start and trace attributes are trusted application configuration.

Place recovery **inside** telemetry (`Use(telemetry)` before `Use(recovery)`) so the server span observes the recovered status. Keep logging from recovery middleware under your application's privacy policy. `WithSpanNameFormatter` receives the registered template and request; do not build names from raw URLs.

## UPSTREAM CAPABILITY DIFFERENCES AND COEXISTENCE

All public `otelgin` v0.72.0 options are represented except provider overrides, which are not applicable. Install this middleware once and do not combine it with direct `otelgin.Middleware` or another HTTP auto-instrumentation layer for the same request.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
