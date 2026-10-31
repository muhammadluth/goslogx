# HTTPROUTER

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/httprouter
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

```go
router := httprouter.New()
router.GET("/orders/:id", goslogxrouter.Handle(logger, "/orders/:id",
    func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
        handleOrder(r.Context(), params.ByName("id"))
    }))
```

Wrap native handlers at registration with the **same static pattern** passed to the router. Parameters are forwarded unchanged through request context; no reflection or parameter-value telemetry is used. Wildcards such as `/files/*filepath` work as static templates. Each registered handler reuses `otelhttp` rather than adding a separate tracing stack.

The server-side std capabilities are preserved: filters, propagator, signal controls, public endpoint functions, span start/name options, static and dynamic attributes, server name, and opt-in body-size message events. Client trace callbacks are not applicable to registered server handlers. Privacy and performance defaults match the std adapter.

Unmatched routes and redirects remain native. For whole-router coverage, the generic standard HTTP wrapper is an alternative, but it cannot infer httprouter's template unless you provide it explicitly. Choose one layer.

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

## UNMATCHED ROUTES

The router bypasses registered-handler instrumentation for 404/405 responses.
Install `Fallback` on the native fallback fields alongside the normal integration:

```go
router.NotFound = goslogxhttprouter.Fallback(logger, nil)
router.MethodNotAllowed = goslogxhttprouter.Fallback(logger, http.HandlerFunc(
    func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusMethodNotAllowed)
    },
))
```

Pass the same signal, filter and propagator options used for matched routes.
`nil` selects the standard 404 handler; pass an existing custom handler to retain
its response behavior. The fallback records one server span and native HTTP metrics
without using the raw request path as a route dimension. Do not put this wrapper
around the entire router: matched routes already have instrumentation. Native
automatic redirects remain outside these fallback handlers.
