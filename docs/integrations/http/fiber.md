# Fiber v3

Import `github.com/muhammadluth/goslogx/plugins/http/fiber` as `goslogxfiber`:

```go
app := fiber.New()
app.Use(goslogxfiber.Middleware(logger, goslogxfiber.WithServerName("payment-api")))
app.Get("/payments/:id", func(c fiber.Ctx) error {
    logger.InfoContext(c.Context(), "", "payments", goslogx.MESSSAGE_TYPE_REQUEST, "received", nil)
    return c.SendStatus(200)
})
```

The plugin wraps official `github.com/gofiber/contrib/v3/otel` v1.3.0 and Fiber v3.5.0. `NewMiddleware` remains an alias API. Use `c.Context()` for logs, queries, Redis and outgoing requests. Upstream middleware extracts incoming context, continues the trace, records registered route templates and applies the application error handler before recording status.

Upstream instruments include `http.server.request.duration`, request/response body sizes and active requests. Private providers remove URL/header/error details while retaining method/status/protocol and template routes. `WithServerName` adds a trusted static server name to spans. `WithRequestFilter` accepts requests to instrument; legacy `WithNext` skips when true. `WithMetricAttributes` accepts fixed environment/version and approved `app.*` labels. `WithPropagator` configures trusted propagation.

The plugin requires Go 1.26 due to the supported current upstream generation. Fiber v2 needs a separate adapter. Register middleware before application routes. Run checks inside `plugins/http/fiber`; root `./...` excludes it.

## HTTP namespace

`github.com/muhammadluth/goslogx/plugins/http/fiber` contains the Fiber v3 implementation and owns its instrumentation scope. `WithTracing(false)` and `WithMetrics(false)` control signals independently. Explicit `WithTraceAttributes`, `WithSpanStartOptions`, `WithSpanNameFormatter` and `WithPublicEndpoint` options complement native route/error behavior. Safe static metric labels also accept `app.channel`, `app.region` and `app.tier`.

See [HTTP integration options](README.md) and the [HTTP examples](../../../examples/http/README.md).
