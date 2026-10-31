# GO-PG V10 COMPATIBILITY

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/pg
```

Import `github.com/muhammadluth/goslogx/plugins/database/pg` as `goslogxpg`:

## QUICK START

```go
if err := goslogxpg.Instrument(db, logger, goslogxpg.WithDBName("payment")); err != nil {
    return err
}
_, err := db.ExecContext(ctx, "SELECT 1")
```

go-pg is maintenance support. Prefer Bun or a maintained database library for new projects. The integration adapts the behavior of the official [`extra/pgotel`](https://github.com/go-pg/pg/tree/v10/extra/pgotel) QueryHook while retaining explicit providers and privacy-safe telemetry.

Client spans inherit the application context. ORM operations use a bounded operation allowlist; raw queries use their parsed operation such as `SELECT`; unknown operations use `QUERY`. The plugin records `db.system.name=postgresql`, `db.operation.name`, optional configured `db.namespace`, non-negative `db.rows_affected`, and generic error status. Not-found/multi-row sentinel results do not count as failures. Endpoint host/port metadata remains available. SQL, arguments, source paths and raw error descriptions remain excluded by default.

`db.client.operation.duration` records seconds with an error dimension on failures. `db.client.connection.count` observes idle/used pool connections through the supplied meter. Use `WithPoolName` to distinguish pools with fixed names (default `pg`). Register once before concurrent use and match provider/database lifetimes. Use `WithTracing` and `WithMetrics` for independent signals. Repeat registration on the same database returns an error.

## UNDERLYING INSTRUMENTATION

This plugin uses the same public go-pg QueryHook extension point as `pgotel`, but does not register `pgotel.NewTracingHook` directly. Upstream `pgotel` captures a package-global tracer and unconditionally emits SQL statements, connection details, database users, source locations, and raw errors; it has no tracer-provider option. The goslogx hook preserves query operations, row counts, endpoint metadata and error classification through the supplied provider. The native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## ADVANCED CONFIGURATION AND PRIVACY

`WithQueryStatement(true)` or `WithStatementCapture(true)` requires
`WithStatementSanitizer(redactSQL)`. go-pg supplies formatted SQL, which can contain bound values.
`WithRawStatement(true)` permits unsanitized diagnostic capture; it can expose user data.
`WithCallerEnabled(true)` adds source metadata. Use `WithTraceAttributes` and
`WithMetricAttributes` to configure span metadata separately from bounded static metric labels.

## LIFECYCLE AND TROUBLESHOOTING

Install once before concurrent database use. Close the database before shutting down its
meter provider. An initialization error leaves no query hook installed; fix the provider or
configuration before retrying. Avoid combining this hook with pgotel on the same database.
