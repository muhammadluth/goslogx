# go-restful v3

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

Uses official `otelrestful` for one server span and propagation. Because that upstream filter supplies tracing only, the shared HTTP policy supplies duration and active-request metrics without adding another span. `SelectedRoutePath()` supplies the template. Construction errors from metric creation are returned by `NewFilter`.

Write responses through `*restful.Response` so native status accounting remains accurate; writing directly through its underlying `ResponseWriter` bypasses go-restful's status bookkeeping. Existing filters and application responses remain in control. Install recovery inside tracing if it must record a recovered failure's final status.

See [HTTP integration options](README.md) for privacy, provider ownership, filtering, signal controls and automatic-instrumentation coexistence. The [HTTP examples](../../../examples/http/README.md) demonstrates downstream correlation.
