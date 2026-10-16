# MongoDB driver v2

```bash
go get github.com/muhammadluth/goslogx/plugins/database/mongo
```

Go 1.26 is required by this module. Examples use `goslogxmongo` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
opts := options.Client().ApplyURI(uri)
if err := goslogxmongo.Instrument(opts, logger,
    goslogxmongo.WithDBName("orders")); err != nil { return err }
client, err := mongo.Connect(opts)
```

Install monitors before `mongo.Connect`. The adapter uses [OpenTelemetry contrib's driver v2 monitor](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo). The pinned contrib revision is a pseudo-version because this new v2 module has no tagged release at the selected revision.

Command monitoring covers find, insert, update, delete, aggregate and transaction commands. Operation names are bounded; unknown commands become `QUERY`. Command documents, filters, replies, server addresses and raw failures are removed before instrumentation sees them. Only an explicitly configured database name is retained. Pool events track idle and used connections without exporting connection IDs. Existing command or pool monitors are rejected; select or compose your instrumentation explicitly.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.
