# BUN

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/bun
```

## QUICK START

Import the plugin as `goslogxbun`. Install it before using the database concurrently:

```go
if err := goslogxbun.Instrument(db, logger,
    goslogxbun.WithDBName("payment"),
); err != nil {
    return err
}
err := db.NewSelect().Model(&payments).Scan(ctx)
```

## UNDERLYING INSTRUMENTATION

The adapter uses [bunotel v1.2.18](https://github.com/uptrace/bun/tree/v1.2.18/extra/bunotel).
Keep Bun, its dialect and bunotel at compatible versions. goslogx injects your providers into
both the query hook and upstream pool instrumentation. It does not install global providers.

## TRACING AND DEFAULT BEHAVIOR

Bun query hooks create client spans under the query context. Ordinary operations retain
names such as `SELECT` and `INSERT`, legacy attributes such as `db.operation` and `db.sql.table`,
and native row counts. Unknown SQL operation text becomes `QUERY` to avoid exporting a malformed
query as the span name. `WithSpanNameFormatter(func(*bun.QueryEvent) string)` overrides names;
return a safe operation label.

Errors retain their original identity for callers. Spans include an exception event and error
classification with a redacted message. Statement and caller capture default off.

## METRICS

Default instrumentation uses native `go.sql.query_timing` as an integer histogram with unit
`milliseconds`, plus upstream `go.sql.*` pool instruments. Query metric dimensions exclude raw
statements and table names. `WithMetricAttributes` adds validated static application dimensions.

`WithCompatibilityMetrics(true)` also enables the previous goslogx
`db.client.operation.duration` histogram in seconds and `db.client.connection.*` observations.
Use this option while migrating dashboards; do not sum both timing instruments.
`WithPoolName` labels these compatibility pool metrics, with default `bun`.

## SUPPORTED OPTIONS AND ADVANCED CONFIGURATION

Use `WithTracing` and `WithMetrics` to control signals independently. `WithDBName` sets a static
namespace. `WithAttributes` forwards safe static native attributes; prefer `WithTraceAttributes`
and `WithMetricAttributes` to distinguish span metadata from metric dimensions.

```go
err := goslogxbun.Instrument(db, logger,
    goslogxbun.WithStatementCapture(true),
    goslogxbun.WithStatementSanitizer(redactSQL),
    goslogxbun.WithCallerEnabled(true),
)
```

`WithQueryStatement(true)` also enables statement capture. Capture requires a sanitizer or
`WithRawStatement(true)`. Bun may format bound values before sanitization; your sanitizer must
handle literals, comments and generated queries. Raw capture can expose credentials and user
data. Caller capture adds source paths and runtime stack inspection.

## LIFECYCLE AND TROUBLESHOOTING

Register once per `*bun.DB`; repeat installation returns an error. Close the database before
shutting down its meter provider, which owns pool callbacks. Construction errors return before
installing the query hook. Use either this adapter or direct bunotel on the same database.

If a dashboard loses data after upgrading, check the native metric names and units above or
enable compatibility metrics during the transition. See the [integration policy](../../architecture/integration-policy.md).
