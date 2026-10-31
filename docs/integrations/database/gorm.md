# GORM

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/gorm
```

Go 1.26 is required by this module. Examples use `goslogxgorm` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
if err := goslogxgorm.Instrument(db, logger,
    goslogxgorm.WithDBName("orders")); err != nil { return err }
err := db.WithContext(ctx).Create(&order).Error
```

The adapter configures the maintained [GORM OpenTelemetry plugin](https://github.com/go-gorm/opentelemetry), supplying explicit providers and filtering telemetry. Create, query, update, delete, raw and row callbacks produce spans and operation metrics. The underlying pool supplies connection metrics. `Transaction(ctx, db, logger, fn, options...)` adds an explicit transaction span while delegating commit/rollback behavior to GORM.

Use an uninstrumented SQL pool underneath this plugin. Alternatively, select SQL-only instrumentation and omit this plugin. Registration rejects an existing GORM OpenTelemetry plugin. GORM's application SQL logger is configured separately; this plugin does not change what that logger prints.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses the upstream GORM OpenTelemetry plugin. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING AND METRICS

Native callback names such as `gorm.Raw` identify CLIENT spans. When GORM has a table name,
upstream can replace the callback name with a query summary such as `SELECT orders`. Database
operation and collection attributes remain available. `ErrRecordNotFound` keeps the upstream non-failure semantics.

`db.client.operation.duration` is a seconds histogram measured at GORM callbacks.
`db.client.connection.count` and `db.client.connection.max` observe the underlying SQL pool.
The upstream global-meter path is disabled so these observations use the supplied provider.

## SUPPORTED OPTIONS AND PRIVACY

Use `WithTracing`, `WithMetrics`, `WithDBName`, `WithTraceAttributes` and `WithMetricAttributes`
for signal and metadata configuration. `WithStatementCapture(true)` requires
`WithStatementSanitizer(redactSQL)`; `WithRawStatement(true)` permits raw diagnostic SQL.
`WithQueryVariables(true)` includes interpolated variables in the text passed to the sanitizer;
it does not itself enable capture. `WithServerAddress(false)` omits native endpoint metadata.

`WithRecordStackTrace(true)` enables the upstream span-end stack option for panics, not a stack
on every ordinary query error. Exception messages stay redacted and panic identity is preserved.
Stack frames can disclose local source paths, so enable them only under an appropriate export policy.

## LIFECYCLE AND TROUBLESHOOTING

Install before serving queries. Close the SQL pool before shutting down telemetry. If spans
appear twice, choose GORM callbacks or instrumented SQL drivers for the same query. An existing
native GORM OTel plugin is rejected. Keep GORM's own SQL logger configuration separate.
