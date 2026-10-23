# NATS PLUGIN EXAMPLE

Run a local NATS server, then `NATS_URL=nats://localhost:4222 go run .` with Go 1.26. The example publishes through the NATS plugin and explicitly processes a received native message. A configured telemetry logger continues producer → remote receipt → processing context. Bodies and subject IDs are not collected as dimensions. See the [NATS guide](../../../docs/integrations/messaging/nats.md) for JetStream and acknowledgement policy.

Consumer snapshots and message metrics belong to the plugin; this example contains no copied instrumentation.

## REQUIREMENTS

Go 1.26 and a NATS server reachable at `NATS_URL`.

## ENVIRONMENT VARIABLES

`NATS_URL` defaults to `nats://localhost:4222`.

## HOW TO RUN

```bash
cd examples/messaging/nats
NATS_URL=nats://localhost:4222 go run .
```

## HOW TO GENERATE TRAFFIC

The executable publishes and consumes its example message during the run.

## EXPECTED TELEMETRY AND VERIFICATION

Expect linked producer, receipt, and process spans plus bounded messaging duration and count metrics. Inspect the configured backend or run plugin tests for in-memory assertions.

## HOW TO SHUT IT DOWN

Stop the process after delivery. Close subscriptions and the NATS connection before telemetry shutdown in production.
