# pgx v5

```bash
go get github.com/muhammadluth/goslogx/plugins/database/pgx
```

Go 1.26 is required by this module. Examples use `goslogxpgx` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
config, err := pgxpool.ParseConfig(dsn)
if err != nil { return err }
if err := goslogxpgx.InstrumentConfig(config, logger,
    goslogxpgx.WithDBName("orders")); err != nil { return err }
pool, err := pgxpool.NewWithConfig(ctx, config)
if err != nil { return err }
defer pool.Close()
if err := goslogxpgx.ObservePool(pool, logger, goslogxpgx.WithDBName("orders")); err != nil { return err }
```

Use `InstrumentConnConfig` before `pgx.ConnectConfig` for a single connection. Public pgx tracer interfaces cover query, batch, copy and connect operations. Native transaction statements are query operations. Existing tracers are rejected, so select one tracing layer before constructing clients.

`ObservePool` reads acquired, idle and maximum connections locally during collection. Match the pool and provider lifetimes. Query parameters, connection strings and table names are excluded.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.
