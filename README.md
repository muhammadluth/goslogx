# goslogx

[![Go Reference](https://pkg.go.dev/badge/github.com/muhammadluth/goslogx.svg)](https://pkg.go.dev/github.com/muhammadluth/goslogx)
[![Go Report Card](https://goreportcard.com/badge/github.com/muhammadluth/goslogx)](https://goreportcard.com/report/github.com/muhammadluth/goslogx)
[![Build Status](https://github.com/muhammadluth/goslogx/workflows/Go/badge.svg)](https://github.com/muhammadluth/goslogx/actions)
[![codecov](https://codecov.io/gh/muhammadluth/goslogx/graph/badge.svg?token=QD1YFY5MC8)](https://codecov.io/gh/muhammadluth/goslogx)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**goslogx** is a high-performance, production-ready structured logging library for Go. Built on top of [Uber's Zap](https://github.com/uber-go/zap), it provides automatic sensitive data masking, compact stack traces, and standardized DTOs for consistent observability across distributed systems.

## ✨ Key Features

- **🔐 Automatic Sensitive Data Masking**
  - Smart JSON body masking for HTTP requests/responses
  - Header masking for Authorization, API keys, tokens
  - Struct field masking with tags (`log:"masked:full"`, `log:"masked:partial"`, `log:"masked:auto"`, and `log:"masked:headers"`)
  - Zero configuration required - works out of the box

- **⚡ High Performance**
  - Zero-allocation design using `sync.Pool` and cached reflection
  - **1-4 allocs/op** for most logging operations
  - **< 2µs/op** for simple logs
  - Efficient JSON masking with minimal overhead

- **📊 Standardized DTOs**
  - Pre-defined schemas for HTTP, Database, and Message Queue logging
  - Consistent field names across your entire infrastructure
  - Easy integration with log aggregation tools (ELK, Grafana, etc.)

- **🎯 Production-Ready**
  - Thread-safe singleton pattern with `sync.Once`
  - Compact single-line stack traces for better searchability
  - Cloud-native severity levels (DEBUG, INFO, WARNING, ERROR, CRITICAL)
  - **93%+ test coverage**

## 📦 Installation

```bash
go get github.com/muhammadluth/goslogx
```

**Requirements:** Go 1.24+

## 🚀 Quick Start

### Basic Usage

```go
package main

import (
    "github.com/muhammadluth/goslogx"
)

func main() {
    // Initialize logger (call once in main)
    goslogx.New(
        goslogx.WithServiceName("payment-service"),
        goslogx.WithMasking(true),
    )

    // Simple logging
    goslogx.Info(
        "trace-123",
        "api-handler",
        goslogx.MESSSAGE_TYPE_EVENT,
        "user login successful",
        nil,
    )

    // Error logging with stack trace
    if err := processPayment(); err != nil {
        goslogx.Error("trace-123", "payment-processor", err)
    }
}
```

## OpenTelemetry Trace Correlation

`goslogx` can own the OpenTelemetry Trace SDK, `TracerProvider`, W3C propagation,
and OTLP/gRPC trace exporter. Applications only import `goslogx`; they do not
need to import or initialize OpenTelemetry packages themselves. OpenTelemetry
dependencies are installed transitively with this library.

Enable OTLP tracing once during application startup:

`SamplingRatio` controls root-trace sampling from `0.0` to `1.0`. Its zero
value uses `goslogx.DefaultSamplingRatio` (`1.0`) for backward-compatible
default behavior. Sampling decisions received from a parent service are always
honored.

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-service"),
    goslogx.WithMasking(true),
    goslogx.WithTracing(goslogx.TracingConfig{
        Endpoint:      "alloy:4317",
        SamplingRatio: 0.1,
        Insecure:      true, // Use TLS for remote production gateways.
        BatchTimeout:       5 * time.Second,
        ExportTimeout:      5 * time.Second,
        MaxQueueSize:       2048,
        MaxExportBatchSize: 512,
        ServiceVersion:        "1.4.2",
        DeploymentEnvironment: "production",
        ServiceInstanceID:     "payment-api-7c9f",
        ResourceAttributes: map[string]string{
            "team.name": "payments",
        },
        ErrorLogInterval: time.Minute,
        // RegisterGlobal: false, // Opt in only when the app requires globals.
    }),
)

if err := logger.TracingError(); err != nil {
    log.Fatal(err)
}

defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = logger.Shutdown(shutdownCtx)
}()

func HandlePayment(ctx context.Context) {
    logger.InfoContext(
        ctx,
        "payment-123",
        "payment-handler",
        goslogx.MessageTypeEvent,
        "payment received",
        map[string]any{
            "status": "received",
        },
    )
}
```

For a TLS-secured remote gateway with authentication:

```go
goslogx.WithTracing(goslogx.TracingConfig{
    Endpoint:      "telemetry.example.com:4317",
    SamplingRatio: 0.1,
    TLSConfig: &tls.Config{
        MinVersion: tls.VersionTLS12,
        RootCAs:    customCAPool,
    },
    Headers: map[string]string{
        "authorization": bearerToken,
    },
    Compression:   "gzip",
    ExportTimeout: 5 * time.Second,
    ServiceVersion:        "1.4.2",
    DeploymentEnvironment: "production",
    ServiceInstanceID:     "payment-api-7c9f",
})
```

`Insecure` cannot be combined with `TLSConfig`. Use it only for a trusted
local Collector. Header values are treated as credentials and are never
included in configuration validation errors.

Batch processing is asynchronous and non-blocking. Defaults are a 2,048-span
queue, batches of up to 512 spans, a 5-second batch timeout, and a 30-second
export timeout. When the queue is full, newly ended spans are dropped to avoid
blocking application traffic. Monitor the Collector and tune queue capacity
for the application's peak telemetry volume. `Shutdown` flushes spans that
remain in the queue.

Tracing requires `WithServiceName`, `ServiceVersion`,
`DeploymentEnvironment`, and `ServiceInstanceID`. These values become the
standard `service.*` and `deployment.environment.name` resource attributes.
`ResourceAttributes` can add deployment metadata such as `team.name`; it cannot
override the required service attributes. Default OpenTelemetry SDK resource
attributes are preserved.

The OTLP/gRPC exporter connects lazily when telemetry is first exported, so a
successful `New` validates configuration but does not guarantee that the
Collector is reachable. Export failures are written through the application
logger and rate-limited by `ErrorLogInterval` (default: one minute).

Use `ForceFlush` when telemetry must be sent before a lifecycle boundary:

```go
flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := goslogx.ForceFlush(flushCtx); err != nil {
    log.Printf("could not flush telemetry: %v", err)
}
```

`Shutdown` is idempotent, flushes pending spans, and prevents new spans from
being created by `StartTrace`. Calls to `ForceFlush` after shutdown return
`goslogx.ErrTracingShutdown`. Always provide a shutdown context with a timeout.

### Provider ownership

`WithTracing` creates a provider owned by `goslogx`. By default it remains
local and does not replace `otel.GetTracerProvider()`. Pass it explicitly to
instrumentation libraries:

```go
handler := otelhttp.NewHandler(
    router,
    "payment-api",
    otelhttp.WithTracerProvider(logger.TracerProvider()),
    otelhttp.WithPropagators(goslogx.TextMapPropagator()),
)
```

Set `RegisterGlobal: true` only when the application explicitly wants the
managed provider installed globally. `Shutdown` closes that managed provider
and restores the previous global provider and propagator.

When the application already owns an SDK, use its provider instead:

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithTracerProvider(existingProvider),
)
```

An external provider is used for `StartTrace` and log correlation, but is never
registered, flushed, or shut down by `goslogx`. Combining `WithTracing` and
`WithTracerProvider` is rejected through `TracingError` because ownership would
be ambiguous.

For Go eBPF/zero-code instrumentation, use `WithTracerProvider` with the
provider selected by the application or agent, and do not enable
`RegisterGlobal`. The OpenTelemetry Auto SDK relies on its global provider;
replacing it manually breaks correlation with eBPF-created spans.

### Metrics ownership

`WithMeterProvider` accepts a `MeterProvider` owned by the application. It is
never registered globally, flushed, or shut down by `goslogx`. When no provider
is supplied (including `WithMeterProvider(nil)`), `goslogx` uses a no-op
provider. This keeps metrics disabled without consulting or changing
`otel.GetMeterProvider()`.

```text
TracerProvider managed:
  lifecycle can be owned by goslogx
TracerProvider external:
  lifecycle is owned by the application
MeterProvider external:
  lifecycle is always owned by the application
```

The application must call `ForceFlush` and `Shutdown` on its own metrics SDK
during application shutdown. Because `goslogx` leaves the global
`MeterProvider` untouched, it can coexist with the provider installed by the
OpenTelemetry Auto SDK or other automatic instrumentation.

Internal logger metrics are opt-in and use the injected provider:

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithMeterProvider(meterProvider),
    goslogx.WithInternalMetrics(true),
)
```

The following instruments describe `goslogx` itself, not application business
activity:

```text
goslogx.log.records
goslogx.telemetry.errors
goslogx.telemetry.force_flush.duration
goslogx.telemetry.force_flush.errors
goslogx.telemetry.shutdown.duration
goslogx.telemetry.shutdown.errors
```

Only the bounded attributes `severity`, `operation`, and `result` are emitted.
Trace IDs, span IDs, request or user IDs, messages, modules, URLs, and error
text are never used as metric attributes. This prevents sensitive data leakage
and unbounded metric cardinality.

The initial internal-metrics performance budget is zero additional allocation
when disabled, and no more than 250 ns/op plus one allocation per log when
enabled (measured as the enabled-minus-disabled delta on the same machine).
Run the focused benchmarks several times and compare the paired results:

```bash
go test -run '^$' -bench='(Info|Error)ContextMetrics|InternalMetric' \
  -benchmem -count=5
```

For the complete ownership matrix, Collector/backend choices, cardinality
rules, graceful shutdown order, and release checklist, see
[`docs/observability-production.md`](docs/observability-production.md).

### Application HTTP RED metrics

Application metrics stay in the application. A copy-and-adapt HTTP middleware
reference is available in [`examples/otel_metrics`](examples/otel_metrics). It
uses the injected provider through `logger.MeterProvider()` and records:

```text
http.server.requests
http.server.errors       # server errors (HTTP 5xx)
http.server.duration     # seconds
payment.events           # example bounded business event
```

Provide the route template from the router after route matching, for example
`/payments/{id}`. Never substitute it with `r.URL.Path`, which could become
`/payments/pay-123` and create an unbounded number of metric series. The
middleware records with `r.Context()`, allowing a supporting metrics SDK to
attach a sampled active span as an exemplar. Business events use a fixed event
name (`payment.created`); do not attach a payment ID as a metric attribute.

### Database pool and NATS consumer metrics

A copy-and-adapt reference is available in
[`examples/otel_db_nats_metrics`](examples/otel_db_nats_metrics). The database
observer receives an application-owned `*sql.DB` and polls `db.Stats()`; it is
not stored in `goslogx`. It records open, in-use, idle, wait-count, and
wait-duration pool metrics. The wait counters are converted from cumulative
`sql.DBStats` values to deltas, so repeated polling does not overcount them.

The NATS reference receives a neutral `NATSConsumerSnapshot` from application
code rather than importing a NATS client. It records pending, ack-pending,
redelivered, lag, publish/consume count, and operation duration metrics using
only these low-cardinality attributes:

```text
messaging.system=nats
messaging.destination.name=payments
messaging.consumer.name=payment-worker
```

Do not use a message ID, payload, subject with generated identifiers, or raw
error text as a metric attribute.

When `InfoContext` or another context API receives a context without an active
span, the log is written normally without trace correlation fields. Logging
does not create orphan spans. Start an operation span explicitly when trace
correlation is required:

```go
ctx, endTrace := goslogx.StartTrace(context.Background(), "process-payment")
defer endTrace()

goslogx.InfoContext(ctx, "payment-123", "payment-handler", goslogx.MessageTypeEvent, "payment received", data)
```

To group multiple logs into one trace, use the library-level lifecycle helper:

```go
ctx, endTrace := goslogx.StartTrace(ctx, "process-payment")
defer endTrace()

goslogx.InfoContext(ctx, "payment-123", "payment", goslogx.MessageTypeEvent, "payment started", nil)
goslogx.WarningContext(ctx, "payment-123", "payment", "provider response was slow", nil)
goslogx.ErrorContext(ctx, "payment-123", "payment", err)
```

### Instrumentation boundaries

Use the official OpenTelemetry instrumentation at network boundaries. It
creates server/client spans and propagates W3C `traceparent` automatically;
`goslogx` only correlates application logs with the active span.

HTTP server and client:

```go
handler := otelhttp.NewHandler(
    router,
    "payment-api",
    otelhttp.WithTracerProvider(logger.TracerProvider()),
    otelhttp.WithPropagators(goslogx.TextMapPropagator()),
)

client := &http.Client{
    Transport: otelhttp.NewTransport(
        http.DefaultTransport,
        otelhttp.WithTracerProvider(logger.TracerProvider()),
        otelhttp.WithPropagators(goslogx.TextMapPropagator()),
    ),
}

// r.Context() contains the inbound server span.
goslogx.InfoContext(r.Context(), "payment-123", "payment-api", goslogx.MessageTypeEvent, "request received", nil)

// Preserve the context so the outbound client span is a child of the server span.
req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, inventoryURL, nil)
```

gRPC server and client use stats handlers:

```go
server := grpc.NewServer(
    grpc.StatsHandler(otelgrpc.NewServerHandler(
        otelgrpc.WithTracerProvider(logger.TracerProvider()),
        otelgrpc.WithPropagators(goslogx.TextMapPropagator()),
    )),
)

conn, err := grpc.NewClient(
    target,
    grpc.WithTransportCredentials(credentials),
    grpc.WithStatsHandler(otelgrpc.NewClientHandler(
        otelgrpc.WithTracerProvider(logger.TracerProvider()),
        otelgrpc.WithPropagators(goslogx.TextMapPropagator()),
    )),
)
```

Always pass the RPC context into logs and downstream operations. For a
goroutine, pass the context explicitly instead of replacing it with
`context.Background()`:

```go
go func(ctx context.Context) {
    goslogx.InfoContext(ctx, "payment-123", "worker", goslogx.MessageTypeEvent, "processing", nil)
}(r.Context())
```

Only launch work with the request context when it must be cancelled with the
request. For detached work, create an explicit background-job span and define
its timeout/lifecycle; do not retain a request context indefinitely.

For databases, Redis, and message brokers, select an instrumentation library
for the exact driver being used and pass the active context to every call
(`QueryContext`, `ExecContext`, Redis command context, and publish/consume
context). Broker producers must inject trace context into message headers and
consumers must extract those headers before creating a consumer span. Verify
the package in the OpenTelemetry registry because support and semantic
conventions differ by driver.

The low-level helpers remain available for custom protocols or libraries that
do not provide instrumentation:

```go
// Incoming request
ctx := goslogx.ExtractHTTPContext(r.Context(), r.Header)
ctx, endTrace := goslogx.StartTrace(ctx, "POST /payments")
defer endTrace()

// Outgoing request
goslogx.InjectHTTPContext(ctx, outboundRequest.Header)
```

Do not combine these helpers with `otelhttp` for the same HTTP boundary;
`otelhttp` already extracts and injects propagation headers.

The package-level `DebugContext`, `InfoContext`, `WarningContext`,
`ErrorContext`, `FatalContext`, `StartTrace`, `Shutdown`, and `TracingError`
functions use the global logger configured by `New`.

A traced log produces output like:

```json
{
  "level": "info",
  "time": "2026-07-29T16:00:00+07:00",
  "msg": "payment received",
  "application_name": "payment-service",
  "obs_trace_id": "c157bd358d20626a78eb4182fb353753",
  "obs_span_id": "6e642e0aa213d5ab",
  "obs_trace_flags": "01",
  "module": "payment-handler",
  "msg_type": "EVENT",
  "severity": "INFO",
  "data": {
    "status": "received"
  }
}
```

The recommended log pipeline is:

```text
goslogx JSON stdout
    → Grafana Alloy
    → Loki
```

Trace export is handled inside `goslogx`:

```text
goslogx OpenTelemetry Trace SDK
    → OTLP
    → Grafana Alloy
    → Tempo
```

### Production Collector reference

A deployable Collector pipeline is available in
[`otel-collector.yaml`](otel-collector.yaml), with operational guidance and
Docker Compose/Kubernetes examples under
[`deployments/otel-collector`](deployments/otel-collector/README.md). It
includes memory limiting, resource enrichment, batching, exporter retry, a
persistent sending queue, health checks, internal Prometheus metrics, optional
tail-sampling policies, and TLS/authentication examples.

This allows a Loki `obs_trace_id` to link to its Tempo trace. Avoid enabling both
`stdout → Alloy → Loki` and `OTLP logs → Alloy → Loki` for the same log stream,
as that would duplicate entries. `goslogx` sends traces—not logs—through OTLP;
logs continue to use only the stdout/writer path. A runnable example requiring
no application-level OpenTelemetry import is available at
[`examples/otel_trace_correlation`](examples/otel_trace_correlation).

### Runtime and infrastructure metrics

CPU, memory, and network traffic are not logger metrics. The source must match
the resource being measured: instrument the Go runtime for heap/GC/goroutines;
use cAdvisor or kubelet for container CPU, memory, and network; and use node
exporter or Collector `hostmetrics` for host-level signals. Collector CPU,
memory, and exporter queue health come from its internal telemetry; log
records/bytes come from the Loki/Alloy pipeline.

Use a metrics backend such as Prometheus, Mimir, or an OTLP metrics service.
Tempo receives traces only. The separate
[`otel-collector-metrics.yaml`](otel-collector-metrics.yaml) template wires
application OTLP metrics and Collector host metrics to that metrics backend.

### HTTP Request/Response Logging with Auto-Masking

```go
// Automatically masks sensitive fields in JSON body and headers
goslogx.Info(
    traceID,
    "api-gateway",
    goslogx.MESSSAGE_TYPE_REQUEST,
    "Incoming request",
    goslogx.HTTPData{
        Method:     "POST",
        URL:        "/api/v1/auth/login",
        StatusCode: 200,
        Headers: goslogx.MaskingLogHttpHeaders("headers", map[string][]string{
            "Authorization": {"Bearer eyJhbGci..."},  // Will be masked: "****"
            "Content-Type":  {"application/json"},    // Not masked
            "X-API-Key":     {"sk_live_123456"},      // Partially masked: "sk****56"
        }),
        Body: goslogx.MaskingLogJSONBytes("body", []byte(`{
            "username": "admin@example.com",  // Partially masked: "ad****om"
            "password": "secret123"           // Fully masked: "****"
        }`)),
        Duration: "125ms",
        ClientIP: "203.0.113.42",
    },
)
```

**Output:**
```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "msg": "Incoming request",
  "application_name": "api-gateway",
  "trace_id": "trace-123",
  "module": "api-gateway",
  "msg_type": "REQUEST",
  "severity": "INFO",
  "data": {
    "method": "POST",
    "url": "/api/v1/auth/login",
    "status_code": 200,
    "headers": {
      "Authorization": ["****"],
      "Content-Type": ["application/json"],
      "X-API-Key": ["sk****56"]
    },
    "body": "{\"username\":\"ad****om\",\"password\":\"****\"}",
    "duration": "125ms",
    "client_ip": "203.0.113.42"
  }
}
```

### Struct Field Masking

```go
type User struct {
    ID       string `json:"id"`
    Email    string `json:"email" log:"masked:partial"`
    Password string `json:"password" log:"masked:full"`
    Name     string `json:"name"`
}

user := User{
    ID:       "user-123",
    Email:    "john.doe@example.com",
    Password: "supersecret",
    Name:     "John Doe",
}

goslogx.Info(traceID, "user-service", goslogx.MESSSAGE_TYPE_EVENT, "user created", user)
// Email: "jo****om", Password: "****", Name: "John Doe" (unchanged)
```

### Masking Tags

Struct fields can opt into one of four masking strategies:

| Tag | Use for | Behavior |
| --- | --- | --- |
| `log:"masked:full"` | Individual sensitive string fields | Replaces the value with `****`. |
| `log:"masked:partial"` | Individual identifiable string fields | Keeps the first and last two characters, for example `jo****om`. |
| `log:"masked:auto"` | Payload fields such as `Body` and `Payload` | Recursively masks nested JSON strings, bytes, maps, structs, pointers, and arrays based on field names. |
| `log:"masked:headers"` | HTTP headers (`map[string][]string`) | Masks header values based on header names, including `Authorization`, `Cookie`, and API-key headers. |

The DTO fields `HTTPData.Body`, `DBData.Payload`, `MQData.Payload`, and `GenericData.Payload` use `masked:auto`. `HTTPData.Headers` uses `masked:headers`. Payload values are copied while masking, so the original application data is not modified.

```go
type Event struct {
    Password string         `json:"password" log:"masked:full"`
    Email    string         `json:"email" log:"masked:partial"`
    Payload  map[string]any `json:"payload" log:"masked:auto"`
    Headers  map[string][]string `json:"headers" log:"masked:headers"`
}
```

## 🔐 Masking Strategies

### Automatic Field Detection

**Full Masking** (completely hidden as `****`):
- `password`, `passwd`, `pwd`
- `secret`, `secret_key`, `api_secret`
- `token`, `authorization`, `bearer`
- `credential`, `private_key`

**Partial Masking** (shows first/last 2 characters):
- `username`, `user_name`, `email`
- `phone`, `mobile`
- `api_key`, `access_key`, `client_id`

### Manual Masking Functions

```go
// Mask JSON string
masked := goslogx.MaskingLogJSONString("data", `{"password":"secret"}`)

// Mask JSON bytes
masked := goslogx.MaskingLogJSONBytes("body", jsonBytes)

// Mask HTTP headers
masked := goslogx.MaskingLogHttpHeaders("headers", headerMap)
```

## 📊 Standardized DTOs

### HTTPData
```go
goslogx.Info(traceID, "api", goslogx.MESSSAGE_TYPE_REQUEST, "request", goslogx.HTTPData{
    Method:     "GET",
    URL:        "/api/v1/users",
    StatusCode: 200,
    Headers:    headers,
    Body:       responseBody,
    Duration:   "45ms",
    ClientIP:   "192.168.1.1",
})
```

### DBData
```go
goslogx.Info(traceID, "database", goslogx.MESSSAGE_TYPE_IN, "query executed", goslogx.DBData{
    Driver:    "postgres",
    Operation: "SELECT",
    Database:  "users_db",
    Table:     "users",
    Statement: "SELECT * FROM users WHERE id = $1",
    Duration:  "12ms",
})
```

### MQData
```go
goslogx.Info(traceID, "messaging", goslogx.MESSSAGE_TYPE_IN, "message received", goslogx.MQData{
    Driver:    "kafka",
    Operation: "consume",
    Topic:     "user-events",
    Group:     "notification-service",
    MessageID: "msg-123",
    Payload:   eventData,
})
```

## ⚡ Performance Benchmarks

Benchmarks run on: `Intel Core i5-12400F @ 2.5GHz, 12 cores`

### Core Logging Operations

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `InfoNoData` | 780 ns | 1 | 24 B |
| `InfoWithDTO` | 1,946 ns | 8 | 296 B |
| `InfoWithNestedMasking` | 1,192 ns | 4 | 64 B |
| `InfoWithSliceMasking` | 1,667 ns | 12 | 288 B |
| `DebugNoData` | 108 ns | 4 | 184 B |
| `DebugWithDTO` | 173 ns | 4 | 184 B |
| `DebugWithNestedMasking` | 188 ns | 4 | 184 B |
| `DebugWithSliceMasking` | 201 ns | 5 | 208 B |
| `WarningNoData` | 1,582 ns | 6 | 449 B |
| `WarningWithDTO` | 3,084 ns | 14 | 770 B |
| `Error` | 2,041 ns | 7 | 465 B |

OpenTelemetry correlation benchmarks (`Apple M1 Pro`, Go 1.24):

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `InfoContextWithSpan` | 4,123 ns | 16 | 1,267 B |
| `InfoContextWithoutSpan` | 3,438 ns | 13 | 1,211 B |
| `ErrorContextWithSpan` | 3,706 ns | 16 | 1,267 B |

Tracing lifecycle benchmarks (`Intel Core i5-12400F`, Linux amd64, Go 1.25
toolchain building the Go 1.24 module; median of three runs):

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `StartTraceDisabled` | 2.2 ns | 0 | 0 B |
| `StartTraceEnabledSampled` | 726 ns | 3 | 552 B |
| `StartTraceEnabledUnsampled` | 283 ns | 3 | 168 B |
| `InfoContextWithoutSpanTracingEnabled` | 4,481 ns | 12 | 1,196 B |

The context APIs perform SpanContext extraction and caller detection in
addition to regular JSON encoding. Run `go test -bench='Context' -benchmem
./...` on your deployment platform for representative results.

Release targets, span-loss tolerance, and the operational checklist are in
[`PRODUCTION_READINESS.md`](PRODUCTION_READINESS.md). Evaluate performance on
comparable, quiet hardware rather than with timing-sensitive unit assertions.

### JSON Masking Performance

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `MaskJSONString_Small` (< 100B) | 2,016 ns | 23 | 1,256 B |
| `MaskJSONString_Medium` (< 1KB) | 8,812 ns | 87 | 4,987 B |
| `MaskJSONString_Large` (> 1KB) | 17,639 ns | 193 | 9,815 B |
| `MaskHttpHeaders` | 1,438 ns | 13 | 560 B |

### Struct Masking Performance

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `StructMasking_Flat` | 1,146 ns | 4 | 56 B |
| `StructMasking_Nested` | 1,243 ns | 5 | 72 B |
| `StructMasking_Slice` | 1,549 ns | 12 | 240 B |

### HTTPData Logging

| Benchmark | Time/op | Allocs/op | Bytes/op |
|-----------|---------|-----------|----------|
| `HTTPData_WithSensitiveData` | 2,151 ns | 10 | 489 B |
| `HTTPData_WithoutSensitiveData` | 1,850 ns | 8 | 417 B |

**Key Takeaways:**
- ✅ **Sub-microsecond** logging for simple operations
- ✅ **Single-digit allocations** for most use cases
- ✅ **Efficient masking** with minimal overhead (< 2µs for small JSON)
- ✅ **Scales well** with data size

Run benchmarks yourself:
```bash
go test -bench=. -benchmem -benchtime=1s
```

## 🎯 Configuration Options

```go
goslogx.New(
    // Set service/application name
    goslogx.WithServiceName("my-service"),
    
    // Enable/disable masking (default: true)
    goslogx.WithMasking(true),
    
    // Set log level (default: Info)
    goslogx.WithDebug(true),  // Enables Debug level
    
    // Custom output writer (default: os.Stdout)
    goslogx.WithOutput(customWriter),
)
```

## 📖 API Documentation

Full API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/muhammadluth/goslogx).

### Core Functions

- `New(...Option)` - Initialize logger with options
- `Info(traceID, module, msgType, msg, data)` - Log informational messages
- `Debug(traceID, module, msgType, msg, data)` - Log debug messages
- `Warning(traceID, module, msg, data)` - Log warnings
- `Error(traceID, module, err)` - Log errors with stack trace
- `Fatal(traceID, module, err)` - Log fatal errors and exit

### Masking Functions

- `MaskingLogJSONString(key, jsonStr)` - Mask sensitive fields in JSON string
- `MaskingLogJSONBytes(key, jsonBytes)` - Mask sensitive fields in JSON bytes
- `MaskingLogHttpHeaders(key, headers)` - Mask sensitive HTTP headers

## 🧪 Testing

```bash
# Run tests
go test ./...

# Final release gates
go test -race ./...
go vet ./...

# Run only the in-process OTLP/gRPC trace and metrics pipeline tests
go test -run 'TestOTLP(Export|Metrics)' ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

The OTLP integration suites start real trace and metrics gRPC receivers on
ephemeral `127.0.0.1` ports. They verify OTLP payloads, failure handling, and
application-owned metrics lifecycle without Docker or an external Collector.
The tests require permission to bind loopback sockets.

For upgrades from `WithTracing()`, `WithOTLPTracing(string)`, or the
intermediate `WithTracing(string)` API, see [`MIGRATION.md`](MIGRATION.md).
The complete HTTP instrumentation and graceful-shutdown example is available
at [`examples/otel_trace_correlation`](examples/otel_trace_correlation/main.go).

**Current Coverage:** 93.5%

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 🙏 Acknowledgments

- Built on top of [Uber's Zap](https://github.com/uber-go/zap) - blazing fast, structured logging
- Inspired by production needs in high-scale distributed systems

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

**Made with ❤️ for the Go community**
