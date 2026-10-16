# Integration privacy and instrumentation layers

New database and messaging modules use the supplied `goslogx.TelemetryProvider`. They never install global providers. Missing tracer/meter capabilities resolve to explicit no-ops. Configure once before concurrent use. Providers and clients belong to the application; close clients before flushing and shutting down owned providers.

## Choose the instrumentation layer

Choose manual goslogx instrumentation **or** automatic instrumentation for a given library. Runtime, compile-time, eBPF and Auto SDK instrumentation can overlap with manual plugins. Enabling both can duplicate spans, propagation and metrics. This includes SQL drivers, Redis, MongoDB, Kafka, RabbitMQ and Cassandra; actual automatic coverage depends on the agent and version you deploy.

The new database and messaging modules expose `WithTracing(false)` and `WithMetrics(false)`. Disable manual traces when an agent owns them; retain metrics only if that agent does not already provide equivalent instruments. Disabling traces still allows messaging context extraction/injection through the configured propagator. Do not assume disabling a manual plugin disables an agent. HTTP and Fiber adapters also expose independent signal controls; see [HTTP integrations](../integrations/http/README.md). Existing Bun, go-pg and Redis have their own configuration; omit those manual integrations where automatic instrumentation owns the same operations.

ORM and SQL instrumentation are also separate layers. For GORM or Ent, select their plugin or the underlying SQL plugin. sqlx deliberately uses only SQL instrumentation. Rewrapping and existing hooks are rejected where detection is reliable; opaque SDK option functions and external agents cannot be detected reliably. There are no runtime heuristics.

## Database privacy

Statements default off. No integration records bind arguments, documents, items, Cypher parameters, request bodies or credentials. Database operation names come from a finite allowlist. `WithDBName` is an application-supplied static namespace, limited to 128 bytes; do not derive it from a request or tenant ID.

SQL, pgx, GORM, Ent, ClickHouse, Cassandra and Neo4j support explicit statement capture:

```go
WithStatementCapture(true),
WithStatementSanitizer(func(statement string) string {
    return redactLiteralsAndIdentifiers(statement) // supply your own policy
}),
```

Capture is rejected without a sanitizer. Sanitizer output is trusted and truncated to 4096 bytes; it can still contain sensitive text if your sanitizer misses it. Bind arguments are never automatically interpolated. Review sanitizers for literal SQL, comments, generated analytical queries and Cypher. MongoDB and DynamoDB expose no document/item capture option. Application and native client logs are configured independently.

Errors retain their original identity for callers. Telemetry records generic `error.type` values and error status without raw exception messages or stack traces. Database duration histograms carry error dimensions; they do not require a separate redundant error counter. SQL's maintained upstream instrumentation supplies its own DBStats/operation metrics. HTTP-based search adapters use HTTP metrics.

## Messaging privacy and cardinality

Headers adapt to OpenTelemetry `propagation.TextMapCarrier`; the provider's propagator handles W3C Trace Context and optional baggage. Configure a composite TraceContext/Baggage propagator on the provider when baggage is needed. Baggage travels over the network and may be sensitive; plugins do not turn it into metric labels.

Payloads, message IDs, keys, offsets, correlation IDs and raw routing keys are never metric dimensions. Destination names default to omission. Opt in with `WithDestinationNormalizer`, returning stable names such as `orders.{id}.created`. Each instance retains at most 64 distinct normalized destinations and uses `_OTHER` after that; names longer than 128 bytes are omitted. Configure one shared instrumentation instance per client rather than constructing one per message. Static consumer/group/client names must also be stable.

Producer spans inject headers into a copy. Consumer processing extracts remote context, records receipt, then creates a child process span around your callback. Success counts use `messaging.client.sent.messages` and `messaging.client.consumed.messages`; timing uses `messaging.client.operation.duration` and `messaging.process.duration` in seconds. Failures are represented by bounded `error.type` dimensions. Receipt is counted once per `Process` call, including redelivery; do not call Process twice for the same delivery stage.

Native acknowledgement, commit, retry and cancellation behavior stays with the client and your callback. Helpers do not automatically acknowledge or commit. Transport completion does not always mean broker persistence: check native publisher confirms, delivery guarantees and return errors for your protocol.
