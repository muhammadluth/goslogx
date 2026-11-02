# GO-RESTFUL V3

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/http/gorestful
```

This optional module requires Go 1.26. Import it with an alias such as the one used below; native framework imports remain in your application.

## QUICK START

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
