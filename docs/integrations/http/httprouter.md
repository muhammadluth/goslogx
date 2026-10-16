# httprouter

```bash
go get github.com/muhammadluth/goslogx/plugins/http/httprouter
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

```go
router := httprouter.New()
router.GET("/orders/:id", goslogxrouter.Handle(logger, "/orders/:id",
    func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
        handleOrder(r.Context(), params.ByName("id"))
    }))
```

Wrap native handlers at registration with the **same static pattern** passed to the router. Parameters are forwarded unchanged through request context; no reflection or parameter-value telemetry is used. Wildcards such as `/files/*filepath` work as static templates. Each registered handler reuses `otelhttp` rather than adding a separate tracing stack.

Unmatched routes and redirects remain native. For whole-router coverage, the generic standard HTTP wrapper is an alternative, but it cannot infer httprouter's template unless you provide it explicitly. Choose one layer.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
