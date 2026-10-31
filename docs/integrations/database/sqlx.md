# SQLX

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/sqlx
```

Go 1.26 is required by this module. Examples use `goslogxsqlx` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
db, err := goslogxsqlx.Open(logger, "postgres", dsn)
if err != nil { return err }
defer db.Close()
err = db.GetContext(ctx, &order, "SELECT id FROM orders WHERE id = $1", id)
```

This is a thin helper over the [database/sql integration](sql.md). `NewDb` accepts an already instrumented SQL pool. There is one SQL instrumentation layer; sqlx does not add another span. Register the driver separately.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses the goslogx database/sql adapter backed by XSAM/otelsql. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING AND DEFAULT BEHAVIOR

Tracing delegates to the SQL plugin and XSAM/otelsql. The sqlx helper adds no equivalent span; SQL statements, transactions and rows keep their driver lifecycle.

## METRICS

Instruments, units and SQL pool observations are exactly those of the [SQL integration](sql.md), including its selected legacy/stable semconv mode.

## SUPPORTED OPTIONS AND ADVANCED CONFIGURATION

Use `WithTracing`, `WithMetrics`, `WithDBName`, `WithTraceAttributes` and `WithMetricAttributes`.
Statement capture is disabled by default. `WithStatementCapture(true)` requires
`WithStatementSanitizer(redactSQL)`; `WithRawStatement(true)` is an explicit sensitive-data opt-in.
See the [integration policy](../../architecture/integration-policy.md) for allowed metric dimensions.

## PRIVACY, LIFECYCLE AND TROUBLESHOOTING

`NewDb` does not retroactively instrument a raw pool. Use an instrumented pool or `Open`, and close rows and the pool normally. Errors retain native identity for the caller; exported failures use concrete types
and redacted messages. Providers remain application-owned.
