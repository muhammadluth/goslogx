# Cassandra / ScyllaDB

```bash
go get github.com/muhammadluth/goslogx/plugins/database/cassandra
```

Go 1.26 is required by this module. Examples use `goslogxcassandra` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
cluster := gocql.NewCluster(hosts...)
if err := goslogxcassandra.InstrumentConfig(cluster, logger,
    goslogxcassandra.WithDBName("orders")); err != nil { return err }
session, err := cluster.CreateSession()
```

Targets `github.com/apache/cassandra-gocql-driver/v2`. Configure observers before creating the session. Query and batch observers retain driver timestamps and native retries; each observed query page/attempt is a client operation. Existing observers are rejected. Bind values, hosts and raw failures are excluded; the keyspace label comes only from `WithDBName`.

To include iterator consumption and late errors, create an observer with `New` and call `observer.Iter(ctx, func(ctx context.Context) error { iter := session.Query(statement).WithContext(ctx).Iter(); /* scan */; return iter.Close() })`. This adds an internal lifecycle span around the callback; the driver observers still record actual database operations. Always close the iterator.

ScyllaDB can use the same CQL driver where its protocol/features are compatible. Check the Apache driver's server support for your deployment; default tests do not certify specific server versions.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.
