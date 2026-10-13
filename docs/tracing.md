# Tracing

Keep using `StartTrace(ctx, name)` for a context and an end function. For native OTel span controls, use `StartSpan` or `Tracer`:

```go
ctx, span := logger.StartSpan(ctx, "payment.process",
    trace.WithSpanKind(trace.SpanKindInternal),
    trace.WithAttributes(attribute.String("payment.partner", "partner-a")),
)
defer span.End()
span.AddEvent("validated")
span.SetAttributes(attribute.String("payment.result", "success"))
span.SetStatus(codes.Ok, "")
```

`logger.Tracer("my-company/payment", trace.WithInstrumentationVersion("1.0"))` selects a custom scope. Call its `Start` with native options for links, timestamps, span kind and attributes. Empty tracer names use `github.com/muhammadluth/goslogx`. Scope names describe code, so keep request IDs, tenants and user IDs out of them. Package-level `Tracer` and `StartSpan` use the global logger.

Call `InfoContext`, `DebugContext`, `WarningContext` or `ErrorContext` with the active context to add `obs_trace_id`, `obs_span_id` and `obs_trace_flags` to logs. The legacy `trace_id` field remains the caller-supplied application identifier. Pass that same context to DB, Redis and HTTP client operations. Existing masking and `StartTrace` behavior remain intact. Application-created spans can record errors and events; sanitize their contents before recording sensitive details.

Plugins obtain the logger's `TextMapPropagator()` and use W3C TraceContext by default. The legacy package-level `TextMapPropagator`, `ExtractHTTPContext` and `InjectHTTPContext` retain W3C trace context plus baggage support. Plugin `WithPropagator` accepts trusted overrides, including baggage if your application needs it; validate baggage privacy first.
