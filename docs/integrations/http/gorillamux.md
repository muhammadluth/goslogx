# GORILLA MUX

## UNDERLYING INSTRUMENTATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/gorillamux
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

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

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
