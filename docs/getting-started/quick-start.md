# QUICK START

Create one logger during application startup. `New` configures the process-wide logger on its first call and returns a `*goslogx.Logger` that can also be passed explicitly to plugins.

```go
package main

import "github.com/muhammadluth/goslogx"

func main() {
    logger := goslogx.New(
        goslogx.WithServiceName("payment-api"),
        goslogx.WithMasking(true),
    )

    logger.Info(
        "business-trace-42",
        "payments",
        goslogx.MESSSAGE_TYPE_EVENT,
        "payment accepted",
        map[string]any{"status": "accepted"},
    )
}
```

Enable managed OTLP tracing and metrics when a Collector is available:

```go
logger := goslogx.New(
    goslogx.WithServiceName("payment-api"),
    goslogx.WithResource(goslogx.ResourceConfig{
        ServiceVersion: "1.2.0",
        DeploymentEnvironment: "production",
    }),
    goslogx.WithTracing(goslogx.TracingConfig{
        Endpoint: "collector:4317",
    }),
    goslogx.WithMetrics(goslogx.MetricsConfig{
        Endpoint: "collector:4317",
    }),
)
```

Check `TracingError()` and `MetricsError()` before accepting traffic. Stop servers and consumers before calling `ForceFlush` and `Shutdown`. See [configuration](configuration.md) and [lifecycle](../guides/lifecycle.md).
