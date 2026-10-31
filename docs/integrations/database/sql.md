# DATABASE/SQL

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/sql
```

Go 1.26 is required by this module. Examples use `goslogxsql` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
db, err := goslogxsql.Open(logger, "mysql", dsn,
    goslogxsql.WithDBName("orders"))
if err != nil { return err }
defer db.Close()
_, err = db.ExecContext(ctx, "UPDATE orders SET status = ? WHERE id = ?", status, id)
```

Import and register your chosen driver separately. `OpenDB` accepts a native `driver.Connector`; `WrapDriver` adapts a driver before pool creation. An existing uninstrumented `*sql.DB` cannot be retrofitted safely. Driver names map to bounded database systems; unrecognized names use `other_sql`.

The adapter uses [XSAM/otelsql](https://github.com/XSAM/otelsql) for query, exec, prepare, transaction and connection spans, operation timing and DBStats metrics. Pool callbacks share the provider lifetime; close the pool before shutting down the provider. SQL comments and argument recording default off.

MySQL (`go-sql-driver/mysql`), PostgreSQL (`lib/pq`), SQLite (`modernc.org/sqlite` or `mattn/go-sqlite3`), SQL Server and other compatible drivers use this same module. Choose one driver in your application; this module adds no database driver or CGO requirement. Driver protocol compatibility still depends on that driver's configuration and server version. Plugin tests exercise standard driver interfaces with an in-memory fake. The [SQL compatibility module](../../../examples/database/sql/compatibility/README.md) executes pure-Go SQLite queries and transactions and checks real MySQL, lib/pq and SQL Server driver construction. These tests do not certify live-server compatibility.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses the maintained XSAM/otelsql instrumentation. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## ADVANCED OPTIONS

`WithSpanOptions(SpanOptions{...})` maps native ping, rows-next events, row-span parenting,
error predicates and lifecycle omission flags. It replaces the default span options, so specify
all flags you need. Defaults enable ping spans, omit row spans and reset-session spans, and
ignore `driver.ErrSkip` as an error.

`WithFilter(func(context.Context, string, string, []driver.NamedValue) bool)` controls native
span creation. `WithSpanNameFormatter(func(context.Context, string, string) string)` receives
the native method and query; return safe operation labels. `WithSQLCommenter(true)` opts into
adding propagated trace context to SQL comments and therefore changes the SQL sent to the server.
These options also work through the sqlx plugin.

Use `WithTracing`, `WithMetrics`, `WithTraceAttributes`, `WithMetricAttributes` and
`WithPropagator` for signal/provider configuration. Statements require a sanitizer with
`WithStatementCapture(true)`, or explicit diagnostic `WithRawStatement(true)`.
The native method names, legacy/stable attributes, rows-next events and error types remain intact.

## DEFAULT BEHAVIOR AND LIFECYCLE

The compatibility defaults enable Ping spans and omit rows and connection-reset spans.
Use `WithSpanOptions` to opt into row lifetimes and RowsNext events; this replaces the complete
span-option configuration. Close rows and transactions normally so their optional spans finish.
Native otelsql instrument names and legacy/stable modes are preserved; SQL text never becomes
a metric dimension. See the [SQL compatibility example](../../../examples/database/sql/compatibility/README.md).

## PRIVACY AND TROUBLESHOOTING

Statements are disabled by default, sanitized with an explicit sanitizer, or raw only through
`WithRawStatement(true)`. `WithSQLCommenter(true)` modifies outgoing SQL to carry context, so
review driver and server support before enabling it. Choose this driver layer or an ORM plugin;
stacking both creates equivalent query spans. The application closes the pool and providers.
