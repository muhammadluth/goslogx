# Neo4j v6

```bash
go get github.com/muhammadluth/goslogx/plugins/database/neo4j
```

Go 1.26 is required by this module. Examples use `goslogxneo4j` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

```go
session, err := goslogxneo4j.WrapSession(
    driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "orders"}),
    logger, goslogxneo4j.WithDBName("orders"))
if err != nil { return err }
defer session.Close(ctx)
result, err := session.Run(ctx, "MATCH (n:Order) RETURN n LIMIT $limit",
    map[string]any{"limit": 10})
if err != nil { return err }
_, err = result.Consume(ctx)
```

Wrap each native session once. Run spans cover result consumption; exhaust, Collect, Single or Consume results, and always close the session. Session closure ends outstanding query spans. Explicit transactions end on Commit/Rollback/Close. Managed ExecuteRead/ExecuteWrite wrap the work callback while the native driver owns retry decisions. Query children inherit the transaction span while retaining the supplied context's cancellation.

Parameters and record values are never inspected. Cypher text requires explicit statement capture and a sanitizer. Session methods that are not overridden remain native; driver-wide connectivity checks and ExecuteQuery helpers are outside this session adapter.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.
