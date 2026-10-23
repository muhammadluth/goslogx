# REDIS EXAMPLE

The [full observability example](../../full-observability/README.md) records a Redis operation in the same trace as HTTP, Bun and NATS. Focused setup:

```go
stop := make(chan struct{})
defer close(stop)
if err := goslogxredis.Instrument(client, logger, goslogxredis.WithCloseChan(stop)); err != nil { return err }
return client.Set(ctx, "order:state", "created", 0).Err()
```

See the [Redis integration guide](../../../docs/integrations/cache/redis.md).

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
