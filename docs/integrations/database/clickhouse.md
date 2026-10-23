# CLICKHOUSE NATIVE V2

```bash
go get github.com/muhammadluth/goslogx/plugins/database/clickhouse
```

Go 1.26 is required by this module. Examples use `goslogxclickhouse` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
conn, err := clickhouse.Open(options)
if err != nil { return err }
db, err := goslogxclickhouse.Wrap(conn, logger)
if err != nil { return err }
if err := db.ObservePool(); err != nil { return err }
err = db.Exec(ctx, "INSERT INTO orders VALUES (?, ?)", id, status)
```

The wrapper preserves the native connection interface and uses `clickhouse.Context` with native `WithSpan` to propagate the client span to ClickHouse. Exec, Select, Query, QueryRow, prepared batches, async inserts and format streams are instrumented. Driver stats provide idle, used and maximum connections.

Query spans remain open until rows are exhausted or closed. Scan a QueryRow, close query/format streams and finalize or close every batch. These native resource lifecycles also end telemetry. Batch append/flush errors are recorded without reading appended values. Query/format payloads and arguments are never captured. For ClickHouse through `database/sql`, use the SQL plugin instead.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses a clickhouse-go connection wrapper with the OpenTelemetry API. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
