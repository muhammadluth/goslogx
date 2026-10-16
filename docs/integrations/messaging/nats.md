# NATS Core and JetStream

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/nats
```

Go 1.26 is required. Wrap an existing native connection; connection configuration remains native.

```go
client, err := goslogxnats.Wrap(nc, logger,
    goslogxnats.WithDestinationNormalizer(func(string) string { return "orders" }))
if err != nil { return err }
err = client.Publish(ctx, "orders.created", payload)
_, err = client.Subscribe(context.Background(), "orders.created",
    func(ctx context.Context, msg *nats.Msg) error {
        return processOrder(ctx, msg.Data)
    })
```

Publish/PublishMsg and Request/RequestMsg create producer spans. Subscribe/QueueSubscribe callbacks extract headers before receipt/process spans. Use `WithErrorHandler` for asynchronous callback failures; it is your application callback and can log under your own privacy policy. Request/reply properties and native timeout behavior remain intact. Instrument reply publication too if you want a reply producer span.

For JetStream, call `WrapJetStream(js, logger, options...)`. Publish/PublishMsg retain native publish options. `Consume(ctx, consumer, handler, options...)` wraps callbacks. `Fetch(ctx, consumer, batch, options...)` records the fetch establishment, then returns the native message batch; call `Process` for each message to trace processing. Fetch timeout/cancellation remains controlled by native Fetch options after the initial call. Native methods not overridden by the wrapper, including async publishing, remain uninstrumented.

```go
batch, err := observed.Fetch(ctx, consumer, 10, jetstream.FetchMaxWait(time.Second))
if err != nil { return err }
for msg := range batch.Messages() {
    if err := observed.Process(ctx, msg, func(ctx context.Context, msg jetstream.Msg) error {
        if err := processOrder(ctx, msg.Data()); err != nil { return err }
        return msg.Ack() // application owns the acknowledgement decision
    }); err != nil { return err }
}
return batch.Error()
```

Shared messaging duration/count/error metrics accompany spans. JetStream metadata supplies a redelivery counter. `ObserveConsumer(ctx, info)` records a caller-refreshed ConsumerInfo snapshot: pending, ack pending and lag (pending + ack pending). It starts no polling goroutine and calls no broker API itself. These NATS-specific metrics use the `messaging.nats.*` namespace, without raw subject or consumer IDs. Fetch's initial span excludes asynchronous waiting/iteration; Process covers actual work.

See [privacy, propagation and coexistence](../../architecture/integration-policy.md) and the [distributed example](../../../examples/full-observability/README.md).
