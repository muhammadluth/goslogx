# GORILLA MUX

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/gorillamux
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

```go
router := mux.NewRouter()
router.Use(goslogxmux.Middleware(logger))
router.HandleFunc("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
    handleOrder(r.Context(), mux.Vars(r)["id"])
}).Methods(http.MethodGet)
```

The plugin wraps official `otelmux` v0.72.0 with matched route templates, explicit providers, propagation, and HTTP metrics. Middleware runs for matched routes; Gorilla's unmatched/not-allowed handlers do not run router middleware.

Tracing and metrics are enabled by default. The plugin exposes request filters, `WithPublicEndpoint`, conditional `WithPublicEndpointFn`, span naming, static trace/metric attributes, bounded `WithMetricAttributesFn`, server name, propagator, and independent signal controls. Headers, bodies, query values, and raw errors remain excluded. Provider overrides are not exposed.

`WithPublicEndpoint` starts a new root with an incoming remote link. `WithSpanNameFormatter` receives the route template/name and request; avoid concrete paths. Dynamic metric callbacks run per request and must return bounded values. Your custom handlers and middleware retain response ownership.

All public `otelmux` v0.72.0 options are represented except provider overrides, which are not applicable. Install this middleware once; do not stack it with direct `otelmux` or a generic HTTP wrapper for the same request.

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
router.NotFoundHandler = goslogxgorillamux.Fallback(logger, nil)
router.MethodNotAllowedHandler = goslogxgorillamux.Fallback(logger, http.HandlerFunc(
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
