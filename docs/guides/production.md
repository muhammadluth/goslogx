# PRODUCTION

Initialize goslogx once, check `TracingError` and `MetricsError`, then attach plugins before starting workers or serving traffic. Use explicit providers with auto-instrumentation and multiple SDKs; leave global registration disabled unless a library requires it. Choose bounded sampling, batch/queue sizes and export deadlines for your workload.

Send both signals to an OTLP Collector. Configure backend routing, credentials, queues and retries there. SDK exporter headers and TLS certificates belong in secret management. Existing [deployment examples](../../deployments/otel-collector/README.md) and the [production operations guide](../operations/production.md) cover Collector infrastructure and application-owned providers.

Stop accepting work before shutdown:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = server.Shutdown(ctx)
_ = logger.ForceFlush(ctx)
_ = logger.Shutdown(ctx)
```

`ForceFlush` flushes managed metrics then traces; `Shutdown` stops managed metrics then traces. SDK shutdown includes their final export. Calls serialize without wait goroutines and waiting callers respect cancellation. Shutdown attempts both providers, joins errors, and returns its stored result on later calls. A failed/timed-out shutdown is terminal; stop work first and budget enough time. External providers remain application-owned and require their own flush/shutdown.

Global registration restores providers only while goslogx still owns the current registration. A replacement installed by another SDK remains in place. Existing tracing registration also restores its propagator/error handler. Close Redis clients and signal `WithCloseChan` to release upstream callback registrations before closing external providers. DB pool callbacks retain their database handle until their provider shuts down; use matching lifetimes.
