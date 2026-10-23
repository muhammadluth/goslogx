# GO-RESTFUL V3

## UNDERLYING INSTRUMENTATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/gorestful
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

```go
container := restful.NewContainer()
filter, err := goslogxrestful.NewFilter(logger)
if err != nil { return err }
container.Filter(filter)
service := new(restful.WebService)
service.Route(service.GET("/orders/{id}").To(
    func(req *restful.Request, resp *restful.Response) {
        handleOrder(req.Request.Context(), req.PathParameter("id"))
        resp.WriteHeader(http.StatusOK)
    }))
container.Add(service)
```

The plugin wraps official `otelrestful` v0.72.0 for one server span and propagation. Because upstream supplies tracing only, the shared HTTP policy supplies duration and active-request metrics without adding another span. `SelectedRoutePath()` supplies the template.

Tracing and metrics are enabled by default. The plugin exposes propagation, request filters, static trace/metric attributes, span-start options, public endpoints and conditional `WithPublicEndpointFn`, server name, and independent signal controls. Provider overrides are not exposed. Headers, bodies, query values, concrete paths, and raw error descriptions are excluded.

All public `otelrestful` v0.72.0 capabilities are represented except provider overrides, which are not applicable. Construction errors from metric creation are returned by `NewFilter`.

Write responses through `*restful.Response` so native status accounting remains accurate; writing directly through its underlying `ResponseWriter` bypasses go-restful's status bookkeeping. Existing filters and application responses remain in control. Install recovery inside tracing if it must record a recovered failure's final status.

Install this filter once. Do not combine it with direct `otelrestful` or another HTTP instrumentation filter for the same request.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
