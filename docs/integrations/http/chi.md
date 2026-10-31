# CHI V5

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/chi
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

```go
router := chi.NewRouter()
router.Use(goslogxchi.Middleware(logger))
router.Get("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
    handleOrder(r.Context(), chi.URLParam(r, "id"))
    w.WriteHeader(http.StatusOK)
})
```

Reuses the standard `otelhttp` layer. Chi resolves nested route patterns during dispatch; this adapter reads `RoutePattern()` after the handler and adds the template to the existing span and metric labeler. Concrete URL parameters are excluded. Install at the root router to observe all requests; subrouter paths combine into the registered template.

The server-side std capabilities are preserved: filters, propagator, signal controls, public endpoint functions, span start/name options, static and dynamic attributes, server name, and opt-in body-size message events. Client trace callbacks are not applicable to router middleware. Privacy and performance defaults match the std adapter.

Choose Chi middleware **or** a generic HTTP wrapper around the router. Nested goslogx standard wrappers recognize their own active instrumentation context and avoid an extra server span, but this does not detect external agents or unrelated middleware. Recovery should execute inside telemetry so the final status is available.

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
