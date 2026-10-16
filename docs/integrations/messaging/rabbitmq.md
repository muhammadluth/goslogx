# RabbitMQ AMQP 0-9-1

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/rabbitmq
```

Go 1.26 is required. The adapter targets RabbitMQ-maintained `github.com/rabbitmq/amqp091-go`.

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

Shared messaging metrics capture successful send/receive counts, operation/process durations and bounded error dimensions. Redelivery and successful settlement counters are available. A `multiple` settlement counts one API call, not an inferred number of broker messages. Native publisher confirms/mandatory returns remain application responsibilities; a successful Publish call does not imply a broker confirmation. The adapter adds no management HTTP API, broker polling, reconnect or retry behavior.

Routing keys default to omission. `WithRoutingKeyNormalizer` can add a normalized routing key to producer spans only; keys are never metric dimensions. Queue/exchange names use `WithDestinationNormalizer` and its bounded destination budget. See [privacy, propagation and coexistence](../../architecture/integration-policy.md).
