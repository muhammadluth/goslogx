# PGX V5

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/pgx
```

Go 1.26 is required by this module. Examples use `goslogxpgx` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

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

Use `InstrumentConnConfig` before `pgx.ConnectConfig` for a single connection. The maintained otelpgx tracer covers query, batch, copy, connect, prepare and pool acquire operations. Native transaction statements are query operations. Existing tracers are rejected, so select one tracing layer before constructing clients.

`ObservePool` uses otelpgx's native `pgxpool.*` instruments for acquired, idle, constructing,
maximum and total connections, acquisition counts and timing, and connection destruction.
These replace the previous `db.client.connection.*` pool metrics. Update dashboard queries;
match the pool and provider lifetimes. Query parameters are excluded by default; credentials remain excluded.
Safe endpoint, database, table and affected-row metadata remain available on spans.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin delegates to [exaring/otelpgx v0.13.0](https://github.com/exaring/otelpgx/tree/v0.13.0). The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## SUPPORTED OPTIONS

`WithTracing` and `WithMetrics` control signals independently. `WithDBName`,
`WithTraceAttributes` and `WithMetricAttributes` add static metadata. Native operation metrics
use `db.client.operation.duration` in seconds and `db.client.operation.errors`.

Use `WithAcquireTracing(false)` to disable native acquire instrumentation,
`WithConnectionDetails(false)` to omit endpoint/database details from the connection,
`WithQuerySpanNamePrefix(true)` for native `query ` / `prepare ` / `batch query ` prefixes,
and `WithSpanNameFormatter(func(context.Context, string) string)` for safe custom operation names.
Custom span names do not change metric operation labels.

## PRIVACY AND LIFECYCLE

Statement capture requires `WithStatementCapture(true)` and `WithStatementSanitizer(redactSQL)`.
`WithRawStatement(true)` permits unsanitized diagnostic SQL; it does not enable bind-argument
capture. Use `WithQueryParameters(redactValues)` to capture sanitized parameter strings, optionally
restricted by `WithQueryParameterFilter`. `WithRawQueryParameters(true)` explicitly permits raw
values. Export is limited to 32 values and 4096 bytes in total. Parameters never become metric labels.
Do not return raw SQL from a span-name formatter.

otelpgx starts spans under a recording parent. goslogx also starts a query span when there is
no recording parent, while retaining native completion and metric handling. Samplers still
control export. Query completion follows pgx's hooks, including result consumption.

## TROUBLESHOOTING

Configure tracing before opening the client. An existing tracer causes a configuration error.
Register pool observations once; close the pool before shutting down its provider.

## ADVANCED CONFIGURATION

`WithPoolStatsInterval(duration)` controls otelpgx's minimum interval between pool-statistics reads.
Prepared statements retain `pgx.prepare_stmt.name`. `WithQueryParameters` receives formatted
parameter strings; its result must satisfy your application's privacy policy. A sanitizer changes
only telemetry and never the arguments sent to PostgreSQL.
