# Bun

Import `github.com/muhammadluth/goslogx/plugins/bun` as `goslogxbun`:

```go
if err := goslogxbun.Instrument(db, logger, goslogxbun.WithDBName("payment")); err != nil {
    return err
}
err := db.NewSelect().Model(&payments).Scan(ctx)
```

Use `WithPoolName` to distinguish pools with fixed names (default `bun`). Register once before concurrent database use. The public Bun QueryHook creates client spans under the supplied context, records generic error status, `db.system.name`, bounded `db.operation.name` and configured `db.namespace`. Unknown operations become `QUERY`. SQL text, parameters, raw errors and credentials stay out of telemetry. `WithQueryStatement(false)` documents the default; `true` returns an unsupported-capture error before registration. `WithAttributes` accepts only a fixed `db.namespace` string.

Metrics include `db.client.operation.duration` in seconds and `db.client.connection.count` by idle/used state. Histogram counts provide operation/error counts. Pool callbacks use the explicit meter and no background plugin goroutine; match database and meter-provider lifetimes.

The integration uses a public Bun QueryHook with explicit providers and excludes query text and raw errors. Keep Bun and dialect module versions identical; this integration uses v1.2.18 for both.
