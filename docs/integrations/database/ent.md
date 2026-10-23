# ENT

```bash
go get github.com/muhammadluth/goslogx/plugins/database/ent
```

Go 1.26 is required by this module. Examples use `goslogxent` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
observedDriver, err := goslogxent.WrapDriver(nativeDriver, logger,
    goslogxent.WithDBName("orders"))
if err != nil { return err }
client := ent.NewClient(ent.Driver(observedDriver))
```

Pass the wrapped `dialect.Driver` to your generated Ent client. Exec/query cover mutations and reads; wrapped transactions retain native behavior and parent queries to the transaction span. No generated entity names or bind values are collected. Choose this layer or an instrumented SQL driver; stacking both creates duplicate query spans. Rewrapping this adapter is rejected.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses an Ent dialect driver wrapper. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
