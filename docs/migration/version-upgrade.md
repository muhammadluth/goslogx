# VERSION UPGRADES

Core and plugins are independent Go modules. Upgrade them deliberately and review each module's `go.mod` changes:

```bash
go get github.com/muhammadluth/goslogx@latest
go get github.com/muhammadluth/goslogx/plugins/http/fiber@latest
go mod tidy
go test ./...
```

When upgrading a plugin, check the target framework/client version, upstream OpenTelemetry instrumentation version, changed defaults, emitted names and attributes, privacy-sensitive options, and automatic-instrumentation coexistence. Validate tracing-only, metrics-only, combined, and disabled configurations used by the application.

Use only canonical domain paths from the [integration index](../integrations/README.md). Replace legacy top-level plugin paths in source and `go.mod` before upgrading.
