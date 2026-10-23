# FIBER V3

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fiber
```

## UNDERLYING INSTRUMENTATION

Import `github.com/muhammadluth/goslogx/plugins/http/fiber` as `goslogxfiber`:

```go
app := fiber.New()
app.Use(goslogxfiber.Middleware(logger, goslogxfiber.WithServerName("payment-api")))
app.Get("/payments/:id", func(c fiber.Ctx) error {
    logger.InfoContext(c.Context(), "", "payments", goslogx.MESSSAGE_TYPE_REQUEST, "received", nil)
    return c.SendStatus(200)
})
```

The plugin wraps official `github.com/gofiber/contrib/v3/otel` v1.3.0 and Fiber v3.5.0. Upstream owns spans, metrics, route handling, error semantics, propagation, and response instrumentation. goslogx supplies providers and privacy policy. `NewMiddleware` remains an alias API.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Client IP, captured headers, trace response headers, and dynamic attributes are disabled. URL paths/queries, user agents, credentials, cookies, and raw errors are removed. Upstream instruments include request duration, body sizes, and active requests.

## SUPPORTED OPTIONS

The adapter exposes `WithNext`/`WithRequestFilter`, propagators and response propagators, trace response header, server name and port, span formatter/start options, public endpoint functions, static and request/response callback attributes, header capture, query redaction, client IP, and independent signal controls. `WithoutMetrics` and deprecated `WithCollectClientIP` remain compatibility aliases.

Dynamic metric callbacks accept only bounded approved application/environment/version attributes. Dynamic trace callbacks reject credential-like and transport semantic keys. Authorization, proxy authorization, Cookie, and Set-Cookie cannot be enabled through header capture. `WithClientIP(true)`, caller-controlled headers, and trace response headers are explicit opt-ins.

## PERFORMANCE CONSIDERATIONS

Request and response callbacks run on every request. Header capture copies configured values. Keep callbacks cheap and the selected attributes bounded. Query redaction is retained as defense in depth even though goslogx does not export `url.query`.

## UPSTREAM CAPABILITY DIFFERENCES AND COEXISTENCE

All public upstream v1.3.0 capabilities are represented except provider overrides, which are not applicable. goslogx changes the upstream client-IP default to off. The plugin requires Go 1.26. Fiber v2 needs a separate adapter. Register the middleware once before application routes and do not combine it with direct Fiber OTel middleware.

## HTTP NAMESPACE

`github.com/muhammadluth/goslogx/plugins/http/fiber` contains the Fiber v3 implementation and owns its instrumentation scope. `WithTracing(false)` and `WithMetrics(false)` control signals independently. Explicit `WithTraceAttributes`, `WithSpanStartOptions`, `WithSpanNameFormatter` and `WithPublicEndpoint` options complement native route/error behavior. Safe static metric labels also accept `app.channel`, `app.region` and `app.tier`.

See [HTTP integration options](README.md) and the [HTTP examples](../../../examples/http/README.md).
