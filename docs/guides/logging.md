# LOGGING

goslogx writes structured JSON logs. Each log includes `application_name`, level, message, module, and the supplied message type or error fields.

```go
logger.Debug(traceID, "payments", goslogx.MESSSAGE_TYPE_EVENT, "validation complete", data)
logger.Info(traceID, "payments", goslogx.MESSSAGE_TYPE_EVENT, "payment accepted", data)
logger.Warning(traceID, "payments", "retrying provider", data)
logger.Error(traceID, "payments", err)
```

Use context-aware methods inside traced work:

```go
logger.DebugContext(ctx, traceID, "payments", goslogx.MESSSAGE_TYPE_EVENT, "validation complete", data)
logger.InfoContext(ctx, traceID, "payments", goslogx.MESSSAGE_TYPE_EVENT, "payment accepted", data)
logger.WarningContext(ctx, traceID, "payments", "retrying provider", data)
logger.ErrorContext(ctx, traceID, "payments", err)
```

## TRACE IDENTIFIERS

`trace_id` is the application or business identifier supplied as the first argument. `obs_trace_id`, `obs_span_id`, and `obs_trace_flags` come from the active OpenTelemetry span in `ctx`.

Do not copy `obs_trace_id` into `trace_id`. Keep business correlation and distributed tracing as separate fields so operators can search by either concept.

## DATA AND MASKING

Pass structured values through `data`. Enable masking and add masking tags before logging credentials or personal data. See [masking](masking.md) and [correlation](correlation.md).
