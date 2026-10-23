# LIFECYCLE

Stop accepting new work before shutting down telemetry. This lets active handlers and consumers finish while exporters are still available.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

_ = server.Shutdown(ctx)
_ = logger.ForceFlush(ctx)
_ = logger.Shutdown(ctx)
```

`ForceFlush` asks goslogx-owned providers to export buffered spans and metrics. `Shutdown` flushes and closes those providers. Application-owned providers supplied with `WithTracerProvider` or `WithMeterProvider` are never shut down by goslogx; the application must flush and close them after all instrumented work has stopped.

Close clients and unregister callbacks that can create telemetry before provider shutdown. Repeated shutdown calls are safe at the goslogx facade, but applications should still use one coordinated shutdown path.
