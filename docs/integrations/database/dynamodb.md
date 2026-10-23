# DYNAMODB / AWS SDK V2

```bash
go get github.com/muhammadluth/goslogx/plugins/database/dynamodb
```

Go 1.26 is required by this module. Examples use `goslogxdynamodb` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
cfg, err := config.LoadDefaultConfig(ctx)
if err != nil { return err }
if err := goslogxdynamodb.Instrument(&cfg, logger); err != nil { return err }
client := dynamodb.NewFromConfig(cfg)
```

Use a config dedicated to DynamoDB and instrument it before client construction. The adapter supplies explicit providers and propagator to [official otelaws middleware](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws), adds operation duration metrics and filters telemetry. SDK credential lookup, signing, retries, pagination and native errors remain unchanged.

GetItem, PutItem, Query, Scan, UpdateItem, DeleteItem, batch and transaction operations are represented by bounded operation names. Items, keys, expressions, table names, endpoints and raw failures are excluded. `WithDBName` supplies an optional static namespace. Installing the same adapter or official OTel middleware twice is rejected when the SDK builds its middleware stack; config registration itself cannot inspect opaque option functions.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## IMPLEMENTATION

This plugin uses the upstream otelaws AWS SDK v2 middleware. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
