# ELASTICSEARCH

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/elasticsearch
```

Pass `goslogxelasticsearch.NewTransport(logger, baseTransport)` into the official client's `Config.Transport`. Go 1.26 is required. The adapter accepts the standard `http.RoundTripper` interface, so it does not import the official client's generated API or constrain its version. Preserve your existing TLS/proxy settings by passing your configured base transport.

## QUICK START

```go
transport := goslogxelasticsearch.NewTransport(logger, http.DefaultTransport)
// Assign transport to the official client's Config.Transport before NewClient.
```

One HTTP client span and HTTP operation metrics cover each request. Names distinguish search, bulk, update, delete and index operations using bounded names. Native client retries can therefore produce multiple HTTP attempt spans; this adapter adds no retry loop. Request bodies, query strings, index names and credentials are not recorded. Responses and errors retain the underlying HTTP plugin's behavior; consume/close response bodies normally.

Do not wrap this transport in another otelhttp/goslogx HTTP transport or enable the official client's tracing simultaneously. See [signal options and coexistence](../../architecture/integration-policy.md). There are no extra database spans or redundant database metrics.

## UNDERLYING INSTRUMENTATION AND TRACING

The transport delegates to the goslogx standard HTTP adapter and official otelhttp. CLIENT spans
cover transport execution through response-body completion. Operation names are derived from
endpoint categories, without collecting index names, document IDs or request bodies.

## METRICS

Native HTTP client duration instruments retain the selected otelhttp version's names, types and
units. There is no separate database-duration recording and no client pool polling.

## DEFAULT BEHAVIOR, OPTIONS AND PRIVACY

Tracing and metrics are enabled when their providers are present. Use `WithTracing`, `WithMetrics`,
`WithPropagator`, `WithTraceAttributes` and `WithMetricAttributes` for independent configuration.
Bounded application dimensions follow the [integration policy](../../architecture/integration-policy.md).
Request-specific query and payload capture is not supported by this transport adapter.

## LIFECYCLE AND TROUBLESHOOTING

Consume and close response bodies so client spans finish. Reuse your application's base transport.
If metrics or spans are duplicated, remove overlapping native-client or HTTP instrumentation.
The adapter does not close the transport or shut down providers.
