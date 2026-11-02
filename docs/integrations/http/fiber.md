# FIBER V3

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fiber
```

## UNDERLYING INSTRUMENTATION

Import `github.com/muhammadluth/goslogx/plugins/http/fiber` as `goslogxfiber`:

## QUICK START

```go
app := fiber.New()
app.Use(goslogxfiber.Middleware(logger, goslogxfiber.WithServerName("payment-api")))
app.Get("/payments/:id", func(c fiber.Ctx) error {
    logger.InfoContext(c.Context(), "", "payments", goslogx.MESSSAGE_TYPE_REQUEST, "received", nil)
    return c.SendStatus(200)
})
```

The plugin wraps official `github.com/gofiber/contrib/v3/otel` v1.3.1 and Fiber v3.6.1. Upstream owns spans, metrics, route handling, error semantics, propagation, and response instrumentation. goslogx supplies providers and privacy policy. `NewMiddleware` remains an alias API.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Client IP, captured headers, trace response headers, and dynamic attributes are disabled. URL paths/queries, user agents, credentials, cookies, and raw errors are removed. Upstream instruments include request duration, body sizes, and active requests.

## SUPPORTED OPTIONS

The adapter exposes `WithNext`/`WithRequestFilter`, propagators and response propagators, trace response header, server name and port, span formatter/start options, public endpoint functions, static and request/response callback attributes, header capture, query redaction, client IP, and independent signal controls. `WithoutMetrics` and deprecated `WithCollectClientIP` remain compatibility aliases.

Dynamic metric callbacks accept only bounded approved application/environment/version attributes. Dynamic trace callbacks reject credential-like and transport semantic keys. Authorization, proxy authorization, Cookie, and Set-Cookie cannot be enabled through header capture. `WithClientIP(true)`, caller-controlled headers, and trace response headers are explicit opt-ins.

## PERFORMANCE CONSIDERATIONS

Request and response callbacks run on every request. Header capture copies configured values. Keep callbacks cheap and the selected attributes bounded. Query redaction is retained as defense in depth even though goslogx does not export `url.query`.

## UPSTREAM CAPABILITY DIFFERENCES AND COEXISTENCE

All public upstream v1.3.1 capabilities are represented except provider overrides, which are not applicable. goslogx changes the upstream client-IP default to off. The plugin requires Go 1.26. Fiber v2 needs a separate adapter. Register the middleware once before application routes and do not combine it with direct Fiber OTel middleware.

## HTTP NAMESPACE

`github.com/muhammadluth/goslogx/plugins/http/fiber` contains the Fiber v3 implementation and owns its instrumentation scope. `WithTracing(false)` and `WithMetrics(false)` control signals independently. Explicit `WithTraceAttributes`, `WithSpanStartOptions`, `WithSpanNameFormatter` and `WithPublicEndpoint` options complement native route/error behavior. Safe static metric labels also accept `app.channel`, `app.region` and `app.tier`.

See [HTTP integration options](README.md) and the [HTTP examples](../../../examples/http/README.md).

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
