# fasthttp

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fasthttp
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

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

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
