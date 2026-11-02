# FASTHTTP

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fasthttp
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

```go
handler, err := goslogxfast.NewHandler(logger,
    func(c *fasthttp.RequestCtx) {
        ctx := goslogxfast.Context(c)
        handleOrder(ctx)
        c.SetStatusCode(fasthttp.StatusOK)
    },
    goslogxfast.WithRoute("/orders/:id"))
if err != nil { return err }
server := &fasthttp.Server{Handler: handler}
```

Uses a small native OpenTelemetry API adapter: it does not convert requests or responses through `net/http`, buffer bodies, or depend on a separate observability backend.

fasthttp itself has no router template API. Wrap a handler at registration and supply its static template with `WithRoute`; omission uses a method-only name without a route label. Never pass `string(c.Path())` as a route. `WithFilter`/`WithRequestFilter` accept `*fasthttp.RequestCtx`.

`Context(c)` exposes the active standard context during the handler; propagate it to logging and downstream calls. The previous context value is restored afterward. Do not retain the pooled RequestCtx or request-owned byte slices for background work. Request cancellation follows native fasthttp semantics, not net/http disconnect semantics.

5xx responses and panics mark failure. Panics are rethrown after telemetry cleanup; your recovery/server policy stays in control. Duration and active-request metrics come from the same shared HTTP policy used for go-restful's missing metrics.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrate downstream correlation.

## UNDERLYING INSTRUMENTATION

This plugin uses a native fasthttp handler adapter using stable handler hooks. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

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
