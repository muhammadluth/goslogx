# HTTP and business metrics

This optional module uses the standard HTTP plugin for request telemetry and adds only an application-owned `orders.created` counter. Run `go test -race ./...` with Go 1.26. There is no second HTTP instrumentation stack or copied status writer. See [HTTP integrations](../../../docs/integrations/http/README.md).
