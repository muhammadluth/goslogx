# SQL plugin example

Run `go run .` with Go 1.26. This example uses the generic SQL plugin with pure-Go SQLite and native transactions, without CGO or a database server. Supply a telemetry-configured logger to `run` to export spans and metrics. Statements remain off and bind values are never recorded. See the [SQL guide](../../../docs/integrations/database/sql.md).

Database instrumentation lives in the plugin; no observer or pool metrics implementation is copied into this example. NATS usage is shown separately in [the NATS example](../../messaging/nats/README.md).
