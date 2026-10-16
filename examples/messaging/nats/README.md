# NATS plugin example

Run a local NATS server, then `NATS_URL=nats://localhost:4222 go run .` with Go 1.26. The example publishes through the NATS plugin and explicitly processes a received native message. A configured telemetry logger continues producer → remote receipt → processing context. Bodies and subject IDs are not collected as dimensions. See the [NATS guide](../../../docs/integrations/messaging/nats.md) for JetStream and acknowledgement policy.

Consumer snapshots and message metrics belong to the plugin; this example contains no copied instrumentation.
