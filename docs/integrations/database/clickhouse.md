# CLICKHOUSE NATIVE V2

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/clickhouse
```

Go 1.26 is required by this module. Examples use `goslogxclickhouse` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
conn, err := clickhouse.Open(options)
if err != nil { return err }
db, err := goslogxclickhouse.Wrap(conn, logger)
if err != nil { return err }
if err := db.ObservePool(); err != nil { return err }
err = db.Exec(ctx, "INSERT INTO orders VALUES (?, ?)", id, status)
```

The wrapper preserves the native connection interface and uses `clickhouse.Context` with native `WithSpan` to propagate the client span to ClickHouse. Exec, Select, Query, QueryRow, prepared batches, async inserts and format streams are instrumented. Driver stats provide idle, used and maximum connections.

Query spans remain open until rows are exhausted or closed. Scan a QueryRow, close query/format streams and finalize or close every batch. These native resource lifecycles also end telemetry. Scan and batch append/flush errors are retained until the cursor or batch finishes. A recoverable scan error does not close a live cursor. Close returns the native driver error even when telemetry records a preceding iteration failure. Query/format payloads and arguments are never captured. For ClickHouse through `database/sql`, use the SQL plugin instead.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses a clickhouse-go connection wrapper with the OpenTelemetry API. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING AND DEFAULT BEHAVIOR

SQL operation names identify CLIENT spans. Query and format streams end on consumption or close; batch spans end on Send, Abort or Close. Native ClickHouse trace context follows the child span.

## METRICS

`db.client.operation.duration` uses seconds. `ObservePool` adds `db.client.connection.count` and `db.client.connection.max` from native connection stats.

## SUPPORTED OPTIONS AND ADVANCED CONFIGURATION

Use `WithTracing`, `WithMetrics`, `WithDBName`, `WithTraceAttributes` and `WithMetricAttributes`.
Statement capture is disabled by default. `WithStatementCapture(true)` requires
`WithStatementSanitizer(redactSQL)`; `WithRawStatement(true)` is an explicit sensitive-data opt-in.
See the [integration policy](../../architecture/integration-policy.md) for allowed metric dimensions.

## PRIVACY, LIFECYCLE AND TROUBLESHOOTING

Close every result and batch. A span that remains open usually indicates an unconsumed cursor or unfinished batch; Scan errors alone do not close live results. Errors retain native identity for the caller; exported failures use concrete types
and redacted messages. Providers remain application-owned.
