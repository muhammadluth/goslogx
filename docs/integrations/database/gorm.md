# GORM

```bash
go get github.com/muhammadluth/goslogx/plugins/database/gorm
```

Go 1.26 is required by this module. Examples use `goslogxgorm` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
if err := goslogxgorm.Instrument(db, logger,
    goslogxgorm.WithDBName("orders")); err != nil { return err }
err := db.WithContext(ctx).Create(&order).Error
```

The adapter configures the maintained [GORM OpenTelemetry plugin](https://github.com/go-gorm/opentelemetry), supplying explicit providers and filtering telemetry. Create, query, update, delete, raw and row callbacks produce spans and operation metrics. The underlying pool supplies connection metrics. `Transaction(ctx, db, logger, fn, options...)` adds an explicit transaction span while delegating commit/rollback behavior to GORM.

Use an uninstrumented SQL pool underneath this plugin. Alternatively, select SQL-only instrumentation and omit this plugin. Registration rejects an existing GORM OpenTelemetry plugin. GORM's application SQL logger is configured separately; this plugin does not change what that logger prints.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses the upstream GORM OpenTelemetry plugin. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
