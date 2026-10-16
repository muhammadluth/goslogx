# Gin

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

Uses official `otelgin` with explicit tracer, meter and propagator. The matched `FullPath()` supplies the route; unmatched requests use a method-only span name and omit the route. Handler `c.Errors` and 5xx responses mark spans as errors; ordinary 4xx responses do not. Native handlers and error/recovery middleware stay in your application.

Place recovery **inside** telemetry (`Use(telemetry)` before `Use(recovery)`) so the server span observes the recovered status. Keep logging from recovery middleware under your application's privacy policy. `WithSpanNameFormatter` receives the registered template and request; do not build names from raw URLs.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
