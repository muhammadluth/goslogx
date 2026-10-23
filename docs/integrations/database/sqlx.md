# SQLX

```bash
go get github.com/muhammadluth/goslogx/plugins/database/sqlx
```

Go 1.26 is required by this module. Examples use `goslogxsqlx` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
db, err := goslogxsqlx.Open(logger, "postgres", dsn)
if err != nil { return err }
defer db.Close()
err = db.GetContext(ctx, &order, "SELECT id FROM orders WHERE id = $1", id)
```

This is a thin helper over the [database/sql integration](sql.md). `NewDb` accepts an already instrumented SQL pool. There is one SQL instrumentation layer; sqlx does not add another span. Register the driver separately.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses the goslogx database/sql adapter backed by XSAM/otelsql. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
