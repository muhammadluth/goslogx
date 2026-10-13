# go-pg v10 compatibility

Import `github.com/muhammadluth/goslogx/plugins/pg` as `goslogxpg`:

```go
if err := goslogxpg.Instrument(db, logger, goslogxpg.WithDBName("payment")); err != nil {
    return err
}
_, err := db.ExecContext(ctx, "SELECT 1")
```

go-pg is maintenance support. Prefer Bun or a maintained database library for new projects. The integration uses a public go-pg v10 QueryHook with explicit providers and privacy-safe telemetry.

Client spans inherit the application context. ORM operations use a bounded operation allowlist; raw query operations use `QUERY`. The plugin records `db.system.name=postgresql`, `db.operation.name`, optional configured `db.namespace` and generic error status. Not-found/multi-row sentinel results do not count as failures. It omits SQL, arguments, connection strings and credentials.

`db.client.operation.duration` records seconds with an error dimension on failures. `db.client.connection.count` observes idle/used pool connections through the supplied meter. Use `WithPoolName` to distinguish pools with fixed names (default `pg`). Register once before concurrent use and match provider/database lifetimes. `WithQueryStatement(false)` preserves privacy; `true` returns a configuration error.
