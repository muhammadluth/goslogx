# BUN EXAMPLE

The [full observability example](../../full-observability/README.md) instruments a real Bun database and executes a query under an HTTP request trace. For the focused setup:

```go
if err := goslogxbun.Instrument(db, logger, goslogxbun.WithDBName("orders")); err != nil { return err }
_, err := db.ExecContext(ctx, "INSERT INTO orders (id) VALUES (?)", id)
return err
```

See the [Bun integration guide](../../../docs/integrations/database/bun.md).

## WHAT THE EXAMPLE DEMONSTRATES

The snippet shows where to attach the plugin while preserving the native client API and application-owned context.

## REQUIREMENTS

Use the target client version declared by the plugin module and a reachable instance of that service.

## ENVIRONMENT VARIABLES

This directory has no standalone executable. Supply connection settings through the host application.

## HOW TO RUN

Copy the snippet into an application that already constructs the native client, then run that application normally.

## HOW TO GENERATE TRAFFIC

Execute the operation shown in the snippet through an active request or worker context.

## EXPECTED TELEMETRY

Expect one plugin-owned operation span, context propagation where the protocol supports it, and the metrics described by the linked integration guide.

## HOW TO VERIFY IT

Inspect the configured trace and metrics backends and confirm parentage under the calling span.

## HOW TO SHUT IT DOWN

Close the native client before flushing and shutting down its telemetry provider.
