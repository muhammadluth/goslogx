# INSTALLATION

Install the core module when an application needs structured logging, masking, custom tracing, custom metrics, or the goslogx telemetry provider contract:

```bash
go get github.com/muhammadluth/goslogx
```

Core requires the Go version declared in [`go.mod`](../../go.mod). Plugins are separate modules and may require a newer Go release. Install only the integration modules used by the application:

```bash
go get github.com/muhammadluth/goslogx/plugins/http/fiber
go get github.com/muhammadluth/goslogx/plugins/database/bun
go get github.com/muhammadluth/goslogx/plugins/cache/redis
go get github.com/muhammadluth/goslogx/plugins/messaging/nats
```

Use the canonical domain-based paths listed in the [integration index](../integrations/README.md). Plugin modules do not need to use the same release number as core; pin and test each module through the application's `go.mod`.

## NEXT STEP

Continue with the [quick start](quick-start.md), then add an [integration](../integrations/README.md).
