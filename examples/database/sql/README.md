# SQL PLUGIN EXAMPLE

Run `go run .` with Go 1.26. This example uses the generic SQL plugin with pure-Go SQLite and native transactions, without CGO or a database server. Supply a telemetry-configured logger to `run` to export spans and metrics. Statements remain off and bind values are never recorded. See the [SQL guide](../../../docs/integrations/database/sql.md).

Database instrumentation lives in the plugin; no observer or pool metrics implementation is copied into this example. NATS usage is shown separately in [the NATS example](../../messaging/nats/README.md).

## REQUIREMENTS

Go 1.26. The pure-Go SQLite driver requires no external database.

## ENVIRONMENT VARIABLES

None.

## HOW TO RUN

```bash
cd examples/database/sql
go run .
```

## HOW TO GENERATE TRAFFIC

The executable performs its SQL operations during startup.

## EXPECTED TELEMETRY AND VERIFICATION

Expect SQL operation spans and DBStats metrics without SQL text or bind values. Run `go test ./...` for in-memory assertions.

## HOW TO SHUT IT DOWN

The example closes the database before returning. A telemetry-enabled host must then flush and shut down its provider.
