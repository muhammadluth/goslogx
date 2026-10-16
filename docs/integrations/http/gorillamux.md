# Gorilla Mux

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

Uses official `otelmux` with matched route templates, explicit providers and HTTP metrics. Middleware runs for matched routes; Gorilla's unmatched/not-allowed handlers do not run router middleware. To observe those as well, use one generic standard HTTP wrapper with static or ServeMux route information instead of stacking both integrations.

`WithPublicEndpoint` starts a new root with an incoming remote link. `WithSpanNameFormatter` receives the route template/name and request; avoid concrete paths. Your custom handlers and middleware retain response ownership.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
