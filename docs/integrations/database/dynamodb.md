# DYNAMODB / AWS SDK V2

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/dynamodb
```

Go 1.26 is required by this module. Examples use `goslogxdynamodb` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
cfg, err := config.LoadDefaultConfig(ctx)
if err != nil { return err }
if err := goslogxdynamodb.Instrument(&cfg, logger); err != nil { return err }
client := dynamodb.NewFromConfig(cfg)
```

Use a config dedicated to DynamoDB and instrument it before client construction. The adapter supplies explicit providers and propagator to [official otelaws middleware](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws), adds operation duration metrics and filters telemetry. SDK credential lookup, signing, retries, pagination and native errors remain unchanged.

GetItem, PutItem, Query, Scan, UpdateItem, DeleteItem, batch and transaction operations are represented by bounded operation names. Items, keys, expressions and raw failures are excluded. Native RPC identity, safe endpoint metadata, table names, index names and consistency settings remain on spans; they are not metric dimensions. `WithDBName` supplies an optional static namespace. Installing the same adapter or official OTel middleware twice is rejected when the SDK builds its middleware stack; config registration itself cannot inspect opaque option functions.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses the upstream otelaws AWS SDK v2 middleware. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING AND METRICS

Native spans retain RPC names such as `DynamoDB.GetItem`, CLIENT kind, `rpc.system.name`
and `rpc.method`, with DynamoDB metadata from otelaws. `db.client.operation.duration` is
an additional seconds histogram over the SDK initialize operation, including its downstream
middleware. It is not a separate span or a per-network-attempt latency measurement.

## SUPPORTED OPTIONS AND ADVANCED CONFIGURATION

`WithTracing`, `WithMetrics`, `WithPropagator`, `WithDBName`, `WithTraceAttributes` and
`WithMetricAttributes` configure the injected providers and bounded dimensions.
`WithAttributeBuilder(func(context.Context, middleware.InitializeInput, middleware.InitializeOutput) []attribute.KeyValue)`
adds operation-specific span metadata alongside the native attribute builder. Sensitive fields
are filtered, and callback output does not become metric dimensions.

## PRIVACY, LIFECYCLE AND TROUBLESHOOTING

Configure before constructing clients and reuse the native SDK credential, retry and timeout
settings. There is no connection pool observation API in this adapter. A missing signal provider
uses a no-op. If instrumentation is rejected during client construction, inspect the config's
APIOptions for a second otelaws installation. Flush the application-owned providers after work ends.
