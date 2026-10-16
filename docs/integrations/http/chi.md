# Chi v5

```bash
go get github.com/muhammadluth/goslogx/plugins/http/chi
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

```go
router := chi.NewRouter()
router.Use(goslogxchi.Middleware(logger))
router.Get("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
    handleOrder(r.Context(), chi.URLParam(r, "id"))
    w.WriteHeader(http.StatusOK)
})
```

Reuses the standard `otelhttp` layer. Chi resolves nested route patterns during dispatch; this adapter reads `RoutePattern()` after the handler and adds the template to the existing span and metric labeler. Concrete URL parameters are excluded. Install at the root router to observe all requests; subrouter paths combine into the registered template.

Choose Chi middleware **or** a generic HTTP wrapper around the router. Nested goslogx standard wrappers recognize their own active instrumentation context and avoid an extra server span, but this does not detect external agents or unrelated middleware. Recovery should execute inside telemetry so the final status is available.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
