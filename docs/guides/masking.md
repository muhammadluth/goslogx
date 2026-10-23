# MASKING

goslogx masking protects data written through goslogx. Enable it with `goslogx.WithMasking(true)` and annotate struct fields:

```go
type LoginEvent struct {
    Password string              `json:"password" log:"masked:full"`
    Email    string              `json:"email" log:"masked:partial"`
    Payload  map[string]any      `json:"payload" log:"masked:auto"`
    Headers  map[string][]string `json:"headers" log:"masked:headers"`
}
```

| Tag | Behavior |
| --- | --- |
| `masked:full` | Replaces the value with `****` |
| `masked:partial` | Keeps a small prefix and suffix |
| `masked:auto` | Recursively masks recognized sensitive field names |
| `masked:headers` | Masks sensitive HTTP header values |

The helpers `MaskingLogJSONString`, `MaskingLogJSONBytes`, and `MaskingLogHttpHeaders` are available when the application needs a masked value before constructing log data.

Masking does not sanitize OpenTelemetry span attributes, metric attributes, third-party instrumentation, or telemetry created directly by the application. Apply a separate allowlist before adding data to traces or metrics. Never record credentials, request bodies, SQL bind values, Redis values, or message payloads as telemetry attributes.
