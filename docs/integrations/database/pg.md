# GO-PG V10 COMPATIBILITY

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/pg
```

Import `github.com/muhammadluth/goslogx/plugins/database/pg` as `goslogxpg`:

```go
if err := goslogxpg.Instrument(db, logger, goslogxpg.WithDBName("payment")); err != nil {
    return err
}
_, err := db.ExecContext(ctx, "SELECT 1")
```

go-pg is maintenance support. Prefer Bun or a maintained database library for new projects. The integration uses a public go-pg v10 QueryHook with explicit providers and privacy-safe telemetry.

Client spans inherit the application context. ORM operations use a bounded operation allowlist; raw query operations use `QUERY`. The plugin records `db.system.name=postgresql`, `db.operation.name`, optional configured `db.namespace` and generic error status. Not-found/multi-row sentinel results do not count as failures. It omits SQL, arguments, connection strings and credentials.

`db.client.operation.duration` records seconds with an error dimension on failures. `db.client.connection.count` observes idle/used pool connections through the supplied meter. Use `WithPoolName` to distinguish pools with fixed names (default `pg`). Register once before concurrent use and match provider/database lifetimes. `WithQueryStatement(false)` preserves privacy; `true` returns a configuration error.

## IMPLEMENTATION

This plugin uses the go-pg QueryHook with the OpenTelemetry API. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
