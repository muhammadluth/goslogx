# Full observability

This nested module runs Fiber → payment.process → Bun → Redis → outbound HTTP with goslogx logs, traces and metrics. The unit test adds an instrumented remote HTTP server and checks every parent span plus log correlation, metric presence and secret exclusion. It uses sqlmock, miniredis and in-memory OTel providers, with no Collector or external database.

```bash
cd examples/full_observability
go test -race ./...
```

For the executable, run PostgreSQL, Redis, an inventory HTTP endpoint and an OTLP/gRPC Collector. It performs `SELECT 1`, so it needs no schema setup. Configure `POSTGRES_DSN`, `REDIS_ADDR`, `INVENTORY_URL` and `OTEL_ENDPOINT`; defaults target local services. Then run `go run .` and `curl -X POST localhost:8080/payments`. The local Collector connection uses plaintext for development; configure TLS before deploying.

The default inventory URL is `http://localhost:8081/health`. Managed trace and metric providers share resource identity and send both signals to the Collector. Signal SIGINT/SIGTERM to stop serving, close clients, flush telemetry and shut down providers with a ten-second deadline.

Go 1.26 is required by Fiber/Redis. Local replace directives resolve the unreleased modules in this checkout; use published core and plugin versions when installing outside this checkout.
