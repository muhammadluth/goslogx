# DATABASE/SQL

```bash
go get github.com/muhammadluth/goslogx/plugins/database/sql
```

Go 1.26 is required by this module. Examples use `goslogxsql` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
db, err := goslogxsql.Open(logger, "mysql", dsn,
    goslogxsql.WithDBName("orders"))
if err != nil { return err }
defer db.Close()
_, err = db.ExecContext(ctx, "UPDATE orders SET status = ? WHERE id = ?", status, id)
```

Import and register your chosen driver separately. `OpenDB` accepts a native `driver.Connector`; `WrapDriver` adapts a driver before pool creation. An existing uninstrumented `*sql.DB` cannot be retrofitted safely. Driver names map to bounded database systems; unrecognized names use `other_sql`.

The adapter uses [XSAM/otelsql](https://github.com/XSAM/otelsql) for query, exec, prepare, transaction and connection spans, operation timing and DBStats metrics. Pool callbacks share the provider lifetime; close the pool before shutting down the provider. SQL comments and argument recording are disabled.

MySQL (`go-sql-driver/mysql`), PostgreSQL (`lib/pq`), SQLite (`modernc.org/sqlite` or `mattn/go-sqlite3`), SQL Server and other compatible drivers use this same module. Choose one driver in your application; this module adds no database driver or CGO requirement. Driver protocol compatibility still depends on that driver's configuration and server version. Plugin tests exercise standard driver interfaces with an in-memory fake. The [SQL compatibility module](../../../examples/database/sql/compatibility/README.md) executes pure-Go SQLite queries and transactions and checks real MySQL, lib/pq and SQL Server driver construction. These tests do not certify live-server compatibility.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses the maintained XSAM/otelsql instrumentation. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
