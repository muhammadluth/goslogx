# HTTP integrations

Each adapter is an optional nested module. Core imports no HTTP framework. Pass a `goslogx.TelemetryProvider`; tracer, meter and propagator capabilities are resolved explicitly with no-op fallbacks. Providers remain application-owned. A single integration instance can serve concurrent requests.

| Integration | Install path below `github.com/muhammadluth/goslogx/` | Implementation |
| --- | --- | --- |
| [net/http](std.md) | `plugins/http/std` | Official otelhttp server/client |
| [Fiber v3](fiber.md) | `plugins/http/fiber` | Official Fiber contrib middleware |
| [Gin](gin.md) | `plugins/http/gin` | Official otelgin |
| [Echo v4](echo.md) | `plugins/http/echo` | Official otelecho |
| [Chi v5](chi.md) | `plugins/http/chi` | Standard HTTP layer plus late RoutePattern |
| [Gorilla Mux](gorillamux.md) | `plugins/http/gorillamux` | Official otelmux |
| [httprouter](httprouter.md) | `plugins/http/httprouter` | Standard HTTP layer at route registration |
| [go-restful v3](gorestful.md) | `plugins/http/gorestful` | Official otelrestful plus missing metrics |
| [fasthttp](fasthttp.md) | `plugins/http/fasthttp` | Native API adapter with shared privacy/metrics |

The standard HTTP module supports Go 1.24; framework adapters require Go 1.26. The canonical HTTP paths are `plugins/http/std` and `plugins/http/fiber`. Fiber implementation and instrumentation scope live in the latter. There is no unsafe reflection between Fiber generations. Fiber v3 is supported; a v2 adapter is not included.

## Options and context

Use `WithServerName` for a static server address/name, `WithRequestFilter` (or `WithFilter` on new adapters) to bypass telemetry, `WithTraceAttributes` for explicit span attributes, and `WithMetricAttributes` for safe static labels. Gin also accepts `WithServiceName` as an alias for its server name. Do not pass request-derived hostnames or identifiers as configuration.

Metric customization accepts string values up to 128 bytes for `app.channel`, `app.region`, `app.tier`, `service.version` and `deployment.environment.name`. Values must be fixed application configuration. Other keys are ignored; request IDs, user IDs and trace IDs are excluded. Span-only attributes are explicit application opt-ins and are not promoted to metric labels. Raw URL, queries, cookies, authorization headers, bodies and exception messages are excluded from automatic telemetry. Application logs and arbitrary custom span attributes remain your responsibility.

`WithTracing(false)` and `WithMetrics(false)` independently disable signals. Missing providers are no-ops and never fall back to OTel globals. `WithPropagator` overrides the provider's propagator when needed; otherwise incoming W3C context follows the provider. Use a composite TraceContext/Baggage propagator if baggage is required. Do not put secrets in baggage.

| Capability | Adapters |
| --- | --- |
| Trace attributes, safe metric labels, filters, signal controls, propagator | All |
| Custom formatter | std, Fiber, Gin, Chi, Mux, httprouter |
| Span start options | std, Fiber, Gin, Chi, httprouter, go-restful, fasthttp |
| Public endpoint root with remote link | std, Fiber, Chi, Mux, httprouter, go-restful |

Formatter signatures follow the native framework or `(template, *http.Request)` where appropriate. They must return stable names. Echo retains its native formatter. fasthttp uses the configured static template. Unsupported upstream-specific options are not exposed as no-ops.

Read the active context from `r.Context()` for net/http routers, `c.Request.Context()` for Gin, `c.Request().Context()` for Echo, `c.Context()` for Fiber v3, and `goslogxfast.Context(c)` for fasthttp. Pass that context to database, Redis, messaging and HTTP clients. `InfoContext` correlates logs through `obs_trace_id` and `obs_span_id`; the legacy `trace_id` argument keeps its existing application correlation meaning.

## Routes and generic fallback

Only registered templates become `http.route` dimensions. ServeMux uses `Request.Pattern`, including a method prefix such as `GET /users/{id}`. Chi resolves nested templates after dispatch. Gin/Echo/Mux/go-restful use native matched-route metadata. httprouter and fasthttp require static registration templates. Unmatched routes omit a route when no template is available; dynamic paths never become a fallback dimension.

Any `http.Handler` router can use the standard wrapper:

```go
handler := goslogxhttp.NewHandler(logger, router, "api")
```

This provides tracing, propagation and metrics but cannot infer every router's template. Use a dedicated adapter for automatic route extraction, or `WithRoute` at individual handler registration. Choose the router adapter **or** the generic wrapper. Do not wrap a raw dynamic path in WithRoute.

`NewTransport` wraps your base transport; nil selects `http.DefaultTransport` without modifying it. `InstrumentClient` returns a shallow copy preserving transport settings, timeout, redirect policy and cookie jar. Consume and close response bodies normally so client spans end.

## Errors and recovery

5xx responses mark server failure; ordinary 4xx responses do not automatically mark an application error. Explicit framework errors can mark failure independently of status. Framework/application handlers retain response ownership. Echo dispatches its configured error handler once before recording the final response; see its guide for handled-error return semantics. Fiber retains its native error handler behavior.

Place recovery inside instrumentation when telemetry must observe the recovered response: telemetry should enter first and finish after recovery. Middleware outside tracing can change status after a span has already ended. fasthttp's adapter marks a panic and rethrows it; it does not replace your recovery policy. Native client/transport failures retain their original errors while telemetry strips raw error text.

## Automatic instrumentation and tests

Use manual goslogx HTTP instrumentation **or** external automatic instrumentation for the same operation. Runtime, compile-time, eBPF and Auto SDK tools may overlap. Disable the manual signals they own or omit middleware; understand both trace and metric ownership before combining them. No global span state or reflection-based agent detection is used. The standard adapter recognizes its own nested server wrapper via request context, but cannot identify every external instrumentation source.

The [HTTP examples](../../../examples/http/README.md) tests Tier 1 frameworks with actual Bun, Redis, NATS and HTTP plugins. Independent adapter tests verify propagation, log correlation, route templates, status/error semantics, custom providers/labels, filters and disabled signals using in-memory OTel providers. The [full example](../../../examples/full-observability/README.md) includes a remote worker. Run `bash scripts/test-modules.sh` to validate each discovered module independently.
