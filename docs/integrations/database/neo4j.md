# NEO4J V6

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/database/neo4j
```

Go 1.26 is required by this module. Examples use `goslogxneo4j` as the plugin import alias and a configured logger as the telemetry provider. Native client imports remain in your application.

## QUICK START

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

Wrap each native session once. Run spans cover result consumption; exhaust, Collect, Single or Consume results, and always close the session. Session closure ends outstanding query spans. Explicit transactions end on Commit/Rollback/Close. Managed ExecuteRead/ExecuteWrite wrap the work callback while the native driver owns retry decisions. Query children inherit the transaction span while retaining the supplied context's cancellation. A managed callback ends its pending query spans before the next retry. Breaking out of `Records` leaves the cursor span open until consumption or session/transaction closure. Native Cypher operation names such as `MATCH`, `CREATE` and `RETURN` identify queries.

Parameters and record values are never inspected. Cypher text requires explicit sanitized capture or `WithRawStatement(true)` for trusted diagnostics. Session methods that are not overridden remain native; driver-wide connectivity checks and ExecuteQuery helpers are outside this session adapter.

Read [privacy and signal options](../../architecture/integration-policy.md) before enabling statement capture or combining instrumentation layers. Providers belong to the application; plugins never shut them down.

## UNDERLYING INSTRUMENTATION

This plugin uses a Neo4j session wrapper. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## TRACING AND DEFAULT BEHAVIOR

CLIENT spans use Cypher operations and native transaction boundaries. Managed retries finish spans from the preceding callback; early iterator exit retains the query span until consumption or close.

## METRICS

`db.client.operation.duration` measures seconds through the relevant result or transaction lifetime. The public session interface does not provide reliable pool statistics.

## SUPPORTED OPTIONS AND ADVANCED CONFIGURATION

Use `WithTracing`, `WithMetrics`, `WithDBName`, `WithTraceAttributes` and `WithMetricAttributes`.
Statement capture is disabled by default. `WithStatementCapture(true)` requires
`WithStatementSanitizer(redactSQL)`; `WithRawStatement(true)` is an explicit sensitive-data opt-in.
See the [integration policy](../../architecture/integration-policy.md) for allowed metric dimensions.

## PRIVACY, LIFECYCLE AND TROUBLESHOOTING

Consume results and finish transactions before closing the session. Use a driver-level integration separately for operations outside the wrapped session. Errors retain native identity for the caller; exported failures use concrete types
and redacted messages. Providers remain application-owned.
