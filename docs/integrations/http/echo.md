# Echo v4

```bash
go get github.com/muhammadluth/goslogx/plugins/http/echo
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

```go
e := echo.New()
e.Use(goslogxecho.Middleware(logger,
    goslogxecho.WithServerName("orders-api")))
e.GET("/orders/:id", func(c echo.Context) error {
    ctx := c.Request().Context()
    return loadOrder(ctx, c.Param("id"))
})
```

Uses official `otelecho` and `c.Path()` templates. The active context is in `c.Request().Context()`. Your configured `HTTPErrorHandler` receives the original handler error and controls the response. The upstream middleware dispatches it before recording status; this adapter consumes that handled error to prevent Echo dispatching it again. Outer middleware therefore sees a handled error as nil. Filtered requests retain Echo's normal error flow.

Do not wrap this adapter in another `otelecho` middleware. Configure recovery inside telemetry when its final response status must be observed. Echo's native span naming is used; no unsupported formatter option is exposed.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
