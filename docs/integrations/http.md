# net/http

Import `github.com/muhammadluth/goslogx/plugins/http` as `goslogxhttp`. Wrap an application handler or existing RoundTripper:

```go
handler := goslogxhttp.NewHandler(logger, handler, "GET payments",
    goslogxhttp.WithRoute("/payments/{id}"),
)
client := &http.Client{
    Timeout: 3*time.Second,
    Transport: goslogxhttp.NewTransport(logger, customTransport),
}
```

Official otelhttp produces server/client spans, W3C propagation and request-duration/body-size histograms. The stable instrumentation scope is `github.com/muhammadluth/goslogx/plugins/http`; upstream instrumentation version and instruments remain intact. `WithRoute` adds a trusted static route template. `WithRequestFilter` skips both traces and metrics when false; `WithSpanNameFormatter` supplies a trusted bounded name. `WithMetricAttributes` accepts fixed environment/version labels. `WithPropagator` overrides the supplied provider propagator.

The wrapper preserves a supplied transport's timeout/proxy/TLS/pooling configuration. Nil selects `http.DefaultTransport`. Close response bodies to finish client spans. It clones requests and disables upstream GetBody replay on the private clone to prevent raw replay errors from reaching the global error handler; it restores GetBody before calling the custom base transport, so application replay behavior remains available.

The attribute boundary drops full URLs, query strings, user agents, headers, client/server addresses and raw error text/events. Application request data and returned errors stay available to handlers/callers. Configure route/name/label values from application constants. otelhttp v0.66.0 exposes no server active-request instrument; the plugin adds no duplicate metric. Run module checks from `plugins/http`, or use `scripts/test-modules.sh`.
