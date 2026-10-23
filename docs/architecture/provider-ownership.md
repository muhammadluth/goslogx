# PROVIDER OWNERSHIP

Provider ownership determines who configures export and who performs shutdown.

## GOSLOGX-OWNED PROVIDERS

`WithTracing` and `WithMetrics` create managed OTLP providers. goslogx applies the shared resource, exposes them to plugins, reports initialization errors, and handles them through `ForceFlush` and `Shutdown`.

## APPLICATION-OWNED PROVIDERS

`WithTracerProvider` and `WithMeterProvider` attach providers created by the application. goslogx and its plugins use them but never register, flush, or shut them down. The application remains responsible for readers, processors, exporters, resources, and shutdown.

Do not configure both managed and external ownership for the same signal. See [lifecycle](../guides/lifecycle.md).
