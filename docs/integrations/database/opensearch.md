# OpenSearch

```bash
go get github.com/muhammadluth/goslogx/plugins/database/opensearch
```

Pass `goslogxopensearch.NewTransport(logger, baseTransport)` into the official client's `Config.Transport`. Go 1.26 is required. The adapter accepts the standard `http.RoundTripper` interface, so it does not import the official client's generated API or constrain its version. Preserve your existing TLS/proxy settings by passing your configured base transport.

```go
transport := goslogxopensearch.NewTransport(logger, http.DefaultTransport)
// Assign transport to the official client's Config.Transport before NewClient.
```

One HTTP client span and HTTP operation metrics cover each request. Names distinguish search, bulk, update, delete and index operations using bounded names. Native client retries can therefore produce multiple HTTP attempt spans; this adapter adds no retry loop. Request bodies, query strings, index names and credentials are not recorded. Responses and errors retain the underlying HTTP plugin's behavior; consume/close response bodies normally.

Do not wrap this transport in another otelhttp/goslogx HTTP transport or enable the official client's tracing simultaneously. See [signal options and coexistence](../../architecture/integration-policy.md). There are no extra database spans or redundant database metrics.
