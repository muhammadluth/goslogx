# RABBITMQ AMQP 0-9-1

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/rabbitmq
```

Go 1.26 is required. The adapter targets RabbitMQ-maintained `github.com/rabbitmq/amqp091-go`.

## QUICK START

```go
observed, err := goslogxrabbitmq.WrapChannel(channel, logger,
    goslogxrabbitmq.WithDestinationNormalizer(func(string) string { return "orders" }))
if err != nil { return err }
err = observed.Publish(ctx, "orders", "created", false, false,
    amqp.Publishing{Body: payload, ContentType: "application/json"})
deliveries, err := observed.Consume(ctx, "orders", "worker", false, false, false, false, nil)
if err != nil { return err }
for delivery := range deliveries {
    err := observed.Process(ctx, "orders", delivery,
        func(ctx context.Context, delivery amqp.Delivery) error {
            if err := processOrder(ctx, delivery.Body); err != nil {
                return delivery.Nack(false, true)
            }
            return delivery.Ack(false)
        })
    if err != nil { return err }
}
```

Publish injects a cloned `amqp.Table` while preserving body, properties, reply-to and correlation ID. Request/reply applications use the same Publish/Process helpers for both directions; there is no additional RPC or correlation-ID telemetry. Consume returns the native delivery channel directly. Process extracts context and decorates the callback delivery's Acknowledger, observing your explicit Ack/Nack/Reject calls without deciding settlement for you. Standalone Ack/Nack/Reject helpers are also available; use one settlement path per delivery.

Shared messaging metrics count successful sends and messages entering processing, including failed handlers, and record operation/process durations with bounded error dimensions. Redelivery and successful settlement counters are available. A `multiple` settlement counts one API call, not an inferred number of broker messages. Native publisher confirms/mandatory returns remain application responsibilities; a successful Publish call does not imply a broker confirmation. The adapter adds no management HTTP API, broker polling, reconnect or retry behavior.

Routing keys default to omission. `WithRoutingKeyNormalizer` can add a normalized routing key to producer spans only; keys are never metric dimensions. Queue/exchange names use `WithDestinationNormalizer` and its bounded destination budget. See [privacy, propagation and coexistence](../../architecture/integration-policy.md).

## UNDERLYING INSTRUMENTATION

This plugin uses an amqp091-go channel wrapper. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.

## DEFAULT BEHAVIOR AND SUPPORTED OPTIONS

Use `WithTracing`, `WithMetrics`, `WithPropagator`, `WithTraceAttributes` and `WithMetricAttributes`
for signal and metadata control. `WithConsumerName`, `WithConsumerGroup` and `WithClientID` accept
static labels. `WithDestinationNormalizer` controls destination dimensions; the routing-key
normalizer is span-only. Both signals are enabled when providers are present; absent providers
use no-ops.

## LIFECYCLE AND TROUBLESHOOTING

The application owns channels, connections, publisher confirms and settlement decisions. Drain
work and finish acknowledgements before provider shutdown. Do not interpret a successful Publish
span as a broker-confirmed delivery. A failed Ack/Nack/Reject remains a native error and is not
counted as a successful settlement. Process uses one processing span, not a synthetic receive span.
