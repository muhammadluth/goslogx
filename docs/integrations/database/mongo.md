# MONGODB DRIVER V2

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/mongo
```

Go 1.26 is required by this module. Examples use `goslogxmongo` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

```go
opts := options.Client().ApplyURI(uri)
if err := goslogxmongo.Instrument(opts, logger,
    goslogxmongo.WithDBName("orders")); err != nil { return err }
client, err := mongo.Connect(opts)
```

Install monitors before `mongo.Connect`. The adapter uses [OpenTelemetry contrib's driver v2 monitor](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo). The pinned contrib revision is a pseudo-version because this new v2 module has no tagged release at the selected revision.

Command monitoring covers find, insert, update, delete, aggregate and transaction commands. Native command names and casing remain intact, together with database, collection and endpoint metadata. Command documents remain excluded from exported telemetry by default; the native monitor can inspect BSON to extract operation metadata. `WithDBName` overrides the native database name when supplied. Pool events track idle and used connections without exporting connection IDs. Existing command or pool monitors are rejected; select or compose your instrumentation explicitly.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses the upstream otelmongo command monitor and native driver pool events. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING, METRICS AND OPTIONS

`WithTracing` and `WithMetrics` control signals independently. Use `WithSpanNameFormatter`
with a `func(*event.CommandStartedEvent) string` callback to supply safe command labels.
`WithTraceAttributes` and `WithMetricAttributes` configure separate static metadata.

The native monitor records `db.client.operation.duration` in seconds. Pool events add
`db.client.connection.count` with idle/used states, without connection IDs or per-connection
addresses as labels. Native command failures retain error classification; exception messages
and status descriptions omit raw server text.

## ADVANCED CONFIGURATION AND PRIVACY

`WithStatementCapture(true)` requires `WithStatementSanitizer(redactCommand)`; the sanitizer
receives the upstream command representation. `WithRawStatement(true)` permits raw BSON
command text and can expose filters and user data. Replies and payload values are not metric
labels. Use safe names in custom formatters.

## LIFECYCLE AND TROUBLESHOOTING

Install monitors once on client options before connecting. The driver matches command starts
and completions by connection/request identity; the application retains client ownership.
Close the client before flushing and shutting down providers. Existing monitors cause an error
so that callers can choose one instrumentation layer.
