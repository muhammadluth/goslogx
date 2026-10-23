# LOG AND TRACE CORRELATION

Context-aware logging reads the active OpenTelemetry span and adds:

- `obs_trace_id`: distributed trace identifier;
- `obs_span_id`: active span identifier;
- `obs_trace_flags`: sampling flags.

```go
ctx, span := logger.StartSpan(ctx, "payment.process")
defer span.End()

logger.InfoContext(ctx, paymentID, "payments", goslogx.MESSSAGE_TYPE_EVENT, "processed", nil)
```

Search logs by `application_name` to select a service and by `obs_trace_id` to find logs for one distributed trace. Keep these values as log fields rather than Loki stream labels because they have very high cardinality. `trace_id` remains the caller-supplied business identifier.

Correlation requires the same `context.Context` to flow through handlers, database calls, messages, and logging calls. See [propagation](propagation.md).
