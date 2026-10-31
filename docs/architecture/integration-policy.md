# INTEGRATION PRIVACY AND INSTRUMENTATION LAYERS

Database and messaging modules use the supplied `goslogx.TelemetryProvider`. They never install global providers. Missing tracer/meter capabilities resolve to explicit no-ops. Configure once before concurrent use. Providers and clients belong to the application; close clients before flushing and shutting down owned providers.

## CHOOSE THE INSTRUMENTATION LAYER

Choose manual goslogx instrumentation **or** automatic instrumentation for a given library. Runtime, compile-time, eBPF and Auto SDK instrumentation can overlap with manual plugins. Enabling both can duplicate spans, propagation and metrics. This includes SQL drivers, Redis, MongoDB, Kafka, RabbitMQ and Cassandra; actual automatic coverage depends on the agent and version you deploy.

Database and messaging modules expose `WithTracing(false)` and `WithMetrics(false)`. Disable manual traces when an agent owns them; retain metrics only if that agent does not already provide equivalent instruments. Disabling traces still allows messaging context extraction/injection through the configured propagator. Do not assume disabling a manual plugin disables an agent. HTTP and Fiber adapters also expose independent signal controls; see [HTTP integrations](../integrations/http/README.md). Bun, go-pg and Redis also expose independent signal controls. Disable equivalent manual signals where automatic instrumentation owns them.

ORM and SQL instrumentation are also separate layers. For GORM or Ent, select their plugin or the underlying SQL plugin. sqlx deliberately uses only SQL instrumentation. Rewrapping and existing hooks are rejected where detection is reliable; opaque SDK option functions and external agents cannot be detected reliably. There are no runtime heuristics.

## DATABASE PRIVACY

Statements default off. Plugins preserve native operation names and safe metadata. Default spans exclude bind values, command documents, request bodies and credentials. Native hook wrappers use bounded SQL/Cypher operation parsing where no upstream naming policy exists. `WithDBName` is an application-supplied static namespace, limited to 128 bytes; do not derive it from a request or tenant ID.

SQL, pgx, Bun, go-pg, GORM, Ent, ClickHouse, Cassandra, Neo4j and MongoDB support explicit statement/command capture:

```go
WithStatementCapture(true),
WithStatementSanitizer(func(statement string) string {
    return redactLiteralsAndIdentifiers(statement) // supply your own policy
}),
```

Capture requires a sanitizer or explicit `WithRawStatement(true)` diagnostic opt-in. Raw capture may expose user data and credentials. Sanitizer output is trusted and truncated to 4096 bytes; it can still contain sensitive text if your sanitizer misses it. Bun and go-pg can format bound values before sanitization; MongoDB capture includes its command representation. Review sanitizers for literal SQL, comments, generated analytical queries and Cypher. DynamoDB exposes no item capture option. Redis has a separate `WithDBStatement` opt-in described in its guide. Application and native client logs are configured independently.

Errors retain their original identity for callers. Telemetry preserves error classification and exception events, with raw messages redacted and stack traces disabled by default. GORM can explicitly enable the native panic stack option; stack frames may expose source paths. Safe native event names and link attributes remain available. Database duration histograms carry error dimensions; they do not require a separate redundant error counter. SQL's maintained upstream instrumentation supplies its own DBStats/operation metrics. HTTP-based search adapters use HTTP metrics.

## MESSAGING PRIVACY AND CARDINALITY

Headers adapt to OpenTelemetry `propagation.TextMapCarrier`; the provider's propagator handles W3C Trace Context and optional baggage. Configure a composite TraceContext/Baggage propagator on the provider when baggage is needed. Baggage travels over the network and may be sensitive; plugins do not turn it into metric labels.

Payloads, message IDs, keys, offsets, correlation IDs and raw routing keys are never metric dimensions. Destination names default to omission. Opt in with `WithDestinationNormalizer`, returning stable names such as `orders.{id}.created`. Each instance retains at most 64 distinct normalized destinations and uses `_OTHER` after that; names longer than 128 bytes are omitted. Configure one shared instrumentation instance per client rather than constructing one per message. Static consumer/group/client names must also be stable.

Producer helpers inject headers into a copy. Native franz-go kotel hooks operate on native records; see the Kafka guide for that mode. Consumer processing extracts remote context and creates one process span around your callback. Receipt counts include handler failures and redelivery. The instruments are `messaging.client.sent.messages` and `messaging.client.consumed.messages`; timing uses `messaging.client.operation.duration` and `messaging.process.duration` in seconds. Failures are represented by bounded `error.type` dimensions. Receipt is counted once per `Process` call, including redelivery; do not call Process twice for the same delivery stage.

Native acknowledgement, commit, retry and cancellation behavior stays with the client and your callback. Helpers do not automatically acknowledge or commit. Transport completion does not always mean broker persistence: check native publisher confirms, delivery guarantees and return errors for your protocol.
