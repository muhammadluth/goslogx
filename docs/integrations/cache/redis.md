# GO-REDIS V9

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/cache/redis
```

## UNDERLYING INSTRUMENTATION

This plugin wraps `github.com/redis/go-redis/extra/redisotel/v9` v9.23.0. `redisotel` owns command, pipeline, dial-span, error, and pool-metric semantics. goslogx supplies providers, a stable scope, lifecycle options, and privacy policy.

```go
closeMetrics := make(chan struct{})
if err := goslogxredis.Instrument(client, logger,
    goslogxredis.WithCloseChan(closeMetrics),
); err != nil {
    return err
}
defer close(closeMetrics)
```

The hook supports Client, ClusterClient, Ring, and UniversalClient values holding those concrete clients. Register it once before concurrent use. Existing and future Ring shards are instrumented; `WithErrorHandler` receives asynchronous initialization errors from future nodes.

## DEFAULT BEHAVIOR

Tracing and metrics are enabled. Known command spans use bounded operation names such as `redis.get` and include `db.operation.name`; unknown/custom commands fall back to `redis.command`, while pipeline and dial spans remain fixed. Keys, values, arguments, server addresses, caller information, statements, and raw error descriptions are omitted. `AUTH` and `HELLO AUTH` use the upstream default filter. `redis.Nil` is not marked as an error. Metrics use `pool.name=redis` and semantic-convention-compliant cumulative counters.

`WithCommandSpanNames(false)` restores the legacy fixed `redis.command` name. Operation mode never includes command arguments, keys, or values.

## SUPPORTED OPTIONS

| Option | Classification | Default | Notes |
| --- | --- | --- | --- |
| `WithTracing` | Pass through | `true` | Enables upstream tracing hooks. |
| `WithMetrics` | Pass through | `true` | Enables upstream pool metrics. |
| `WithDBSystem` | Safe wrapper | `redis` | Blank values fall back to `redis`. |
| `WithAttributes` | Safe wrapper | none | Static attributes; sensitive-looking keys are rejected. Keep values bounded because they also reach metrics. |
| `WithPoolName` | Safe wrapper | `redis` | Blank values fall back to `redis`. Keep the value bounded. |
| `WithDBStatement` | Disabled by default | `false` | Raw command capture can expose keys and values. |
| `WithCommandSpanNames` | Safe wrapper | `true` | Adds only the bounded operation name; set `false` for legacy `redis.command`. |
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

All public redisotel v9.23.0 configuration capabilities are represented. Provider overrides are not applicable because goslogx owns provider selection. goslogx additionally normalizes span names and removes raw error details while preserving error status.

## AUTO-INSTRUMENTATION COEXISTENCE

Do not call `redisotel.InstrumentTracing` or `redisotel.InstrumentMetrics` directly on a client already instrumented by goslogx. Installing both produces duplicate hooks and duplicate telemetry.
