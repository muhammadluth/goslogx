# GO-REDIS V9

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/cache/redis
```

## UNDERLYING INSTRUMENTATION

This plugin wraps `github.com/redis/go-redis/extra/redisotel/v9` v9.23.0. `redisotel` owns command, pipeline, dial-span, error, and pool-metric semantics. goslogx supplies providers, a stable scope, lifecycle options, and privacy policy.

## QUICK START

```go
closeMetrics := make(chan struct{})
if err := goslogxredis.Instrument(client, logger,
    goslogxredis.WithCloseChan(closeMetrics),
); err != nil {
    return err
}
defer close(closeMetrics)
```

The hook supports Client, ClusterClient, Ring, and UniversalClient values holding those concrete clients. Register it once before concurrent use. Repeated calls on the same client reuse the first registration result and configuration. Existing and future Ring shards are instrumented; `WithErrorHandler` receives asynchronous initialization errors from future nodes.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Native command spans retain names such as `get`, `set`, `mget`, and module commands. Pipeline spans retain the native `redis.pipeline ...` summary; dial spans use `redis.dial`. Safe server addresses and ports remain available. Connection-string attributes retain only the endpoint. Keys, values, arguments, caller information, statements, and raw error descriptions are omitted by default. `AUTH` and `HELLO AUTH` use the upstream default filter. `redis.Nil` is not marked as an error. Metrics use `pool.name=redis` and semantic-convention-compliant cumulative counters.

`WithCommandSpanNames(false)` restores the legacy fixed `redis.command` name. This compatibility option is deprecated. Normal native command names do not contain arguments.

## SUPPORTED OPTIONS

| Option | Classification | Default | Notes |
| --- | --- | --- | --- |
| `WithTracing` | Pass through | `true` | Enables upstream tracing hooks. |
| `WithMetrics` | Pass through | `true` | Enables upstream pool metrics. |
| `WithDBSystem` | Safe wrapper | `redis` | Blank values fall back to `redis`. |
| `WithAttributes` | Safe wrapper | none | Static attributes; sensitive-looking keys are rejected. Keep values bounded because they also reach metrics. |
| `WithPoolName` | Safe wrapper | `redis` | Blank values fall back to `redis`. Keep the value bounded. |
| `WithDBStatement` | Disabled by default | `false` | Raw command capture can expose keys and values. |
| `WithCommandSpanNames` | Safe wrapper | `true` | Preserves native names; deprecated `false` selects legacy `redis.command`. |
| `WithCallerEnabled` | Disabled by default | `false` | Adds runtime stack discovery and source attributes. |
| `WithCommandFilter` | Safe wrapper | upstream default | Returning `true` skips the command span. The AUTH safety filter always remains active. |
| `WithCommandsFilter` | Safe wrapper | upstream default | Returning `true` skips the pipeline span. Pipelines containing authentication commands always remain filtered. |
| `WithDialFilter` | Pass through | `false` | Follows redisotel semantics: `true` suppresses dial spans. |
| `WithCloseChan` | Pass through | `nil` | Releases upstream metric callback registrations when closed. |
| `WithSemConvCompliantMetrics` | Pass through | `true` | Set `false` only for legacy UpDownCounter compatibility. |
| `WithErrorHandler` | goslogx lifecycle | `nil` | Reports asynchronous Ring-node initialization failures. |

Tracer and meter provider overrides are deliberately unavailable. Providers always come from the supplied `goslogx.TelemetryProvider`.

## EXPORTED METRICS

Redis metrics are exported to the configured metric backend; they do not appear as child spans in a trace waterfall. The upstream integration emits:

- `db.client.connections.idle.max`
- `db.client.connections.idle.min`
- `db.client.connections.max`
- `db.client.connections.usage`
- `db.client.connections.waits`
- `db.client.connections.waits_duration`
- `db.client.connections.timeouts`
- `db.client.connections.hits`
- `db.client.connections.misses`
- `db.client.connections.create_time`
- `db.client.connections.use_time`

Query these instruments in Mimir, Prometheus, or the Grafana metrics explorer. Traces and metrics share the same goslogx OpenTelemetry resource, so `service.name`, `service.version`, `service.instance.id`, and `deployment.environment.name` remain consistent across both signals.

## PRIVACY DEFAULTS

Statement and caller capture are disabled. Enabling statement capture can record Redis keys and values. Custom attribute names are validated, but applications remain responsible for bounded, non-sensitive values.

## PERFORMANCE CONSIDERATIONS

Caller capture performs runtime stack discovery for every observed command or pipeline. Statement capture formats commands. Both remain opt-in. Metrics register callbacks; close `WithCloseChan` during shutdown when the meter provider outlives the Redis client.

## UPSTREAM CAPABILITY DIFFERENCES

All public redisotel v9.23.0 configuration capabilities are represented. Provider overrides are not applicable because goslogx owns provider selection. goslogx preserves native names, events, safe link attributes and error classification while redacting raw error details.

## AUTO-INSTRUMENTATION COEXISTENCE

Do not call `redisotel.InstrumentTracing` or `redisotel.InstrumentMetrics` directly on a client already instrumented by goslogx. Installing both produces duplicate hooks and duplicate telemetry.
