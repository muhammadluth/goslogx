# Optional integrations

Each plugin is a nested Go module. Installing core does not resolve Fiber, Bun, go-pg or Redis dependencies.

```bash
go get github.com/muhammadluth/goslogx
# After the prerequisite core and plugin releases are published:
go get github.com/muhammadluth/goslogx/plugins/fiber
go get github.com/muhammadluth/goslogx/plugins/http
go get github.com/muhammadluth/goslogx/plugins/bun
go get github.com/muhammadluth/goslogx/plugins/pg
go get github.com/muhammadluth/goslogx/plugins/redis
```

| Module | Integration | Minimum Go |
| --- | --- | --- |
| [fiber](integrations/fiber.md) | Official Fiber v3 middleware | 1.26 |
| [http](integrations/http.md) | Official otelhttp handler/transport | 1.24 |
| [bun](integrations/bun.md) | Privacy-safe public QueryHook | 1.24 |
| [pg](integrations/pg.md) | go-pg v10 compatibility QueryHook | 1.24 |
| [redis](integrations/redis.md) | Official redisotel hooks | 1.26 |

Pass a `goslogx.TelemetryProvider`, such as the configured logger. Plugins use explicit tracer/meter providers and the propagator where relevant. Missing providers use no-ops; tracing-only and metrics-only configurations work. Plugins do not shut down providers or mutate OTel globals. Register instrumentation once, before concurrent application use; repeated registration adds duplicate hooks.

See [full observability](../examples/full_observability/README.md) for Fiber, a Bun query, Redis and an outbound HTTP call in one trace. go-pg remains maintenance support; prefer Bun or another maintained driver for new projects.

## Author a plugin

Accept this small consumer contract:

```go
type TelemetryProvider interface {
    TracerProvider() trace.TracerProvider
    MeterProvider() metric.MeterProvider
    TextMapPropagator() propagation.TextMapPropagator
}
```

Keep your target library in a separate module. Validate the target client, default missing capabilities to explicit no-ops, and configure upstream instrumentation with providers. Use the propagator only at network boundaries. Retain the caller's context and custom transport/client configuration.

```go
func Instrument(client *SomeClient, provider goslogx.TelemetryProvider, opts ...Option) error {
    if client == nil { return errors.New("myplugin: nil client") }
    // Resolve providers with explicit no-op defaults; do not read global providers.
    // Configure the target's official hooks using these providers and safe options.
    return nil
}
```

Use `type Option func(*config)` with matching concepts such as `WithDBName`, `WithRequestFilter` or `WithPropagator`. Use `github.com/your-org/goslogx-myplugin` as a stable scope. If wrapping upstream instrumentation, scope provider adapters can select the plugin scope while retaining upstream instrumentation versions. Follow current OTel semantic conventions. Bound operation/route/error dimensions, exclude secrets before collecting telemetry and let the application own provider lifecycles. Test with in-memory exporters/readers, assert parent relationships and privacy, and add your module to validation. Keep dependency versions compatible with the core and target library.
