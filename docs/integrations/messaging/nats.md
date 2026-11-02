# NATS CORE AND JETSTREAM

## INSTALLATION

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/nats
```

Go 1.26 is required. Wrap an existing native connection; connection configuration remains native.

## QUICK START

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

Publish/PublishMsg and Request/RequestMsg create producer spans. Subscribe/QueueSubscribe callbacks extract headers before creating a process span. Use `WithErrorHandler` for asynchronous callback failures; it is your application callback and can log under your own privacy policy. Request/reply properties and native timeout behavior remain intact. Instrument reply publication too if you want a reply producer span.

For JetStream, call `WrapJetStream(js, logger, options...)`. Publish/PublishMsg retain native publish options. `Consume(ctx, consumer, handler, options...)` wraps callbacks. `Fetch(ctx, consumer, batch, options...)` records the complete native batch delivery and returns a forwarding batch with native message/error identity; call `Process` for each message to trace processing. Fetch timeout/cancellation remains controlled by native Fetch options after the initial call. Native methods not overridden by the wrapper remain uninstrumented; async publishing is covered as described below.

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

Shared messaging duration/count/error metrics accompany spans. JetStream metadata supplies a redelivery counter. `ObserveConsumer(ctx, info)` records a caller-refreshed ConsumerInfo snapshot: pending, ack pending and lag (pending + ack pending). It starts no polling goroutine and calls no broker API itself. These NATS-specific metrics use the `messaging.nats.*` namespace, without raw subject or consumer IDs. Fetch spans include native asynchronous delivery through channel closure and final batch error; Process covers application work. The forwarding channel matches the native buffer capacity and does not acknowledge messages.

See [privacy, propagation and coexistence](../../architecture/integration-policy.md) and the [distributed example](../../../examples/full-observability/README.md).

## LIFECYCLE

The application owns subscriptions, acknowledgements, JetStream consumers, and NATS connections. Drain or close them before flushing and shutting down the telemetry provider so final producer and consumer operations can export.

## SERVERS WITHOUT HEADER SUPPORT

Core Publish and Request omit telemetry headers when the server does not support them, so
payload delivery still works. Consumers then start their own process trace because no remote
context crossed the network. If your application supplies headers, the native client retains
its `ErrHeadersNotSupported` error; goslogx does not discard application headers.

## UNDERLYING INSTRUMENTATION, OPTIONS AND PRIVACY

This adapter uses native NATS Core and JetStream APIs. `WithTracing` and `WithMetrics` independently
control signals; `WithPropagator` configures header injection and extraction. `WithConsumerName`,
`WithConsumerGroup`, `WithClientID`, `WithTraceAttributes` and `WithMetricAttributes` add configured
metadata. Payloads and raw error messages are not exported. Destination normalization applies
before bounded metric labels are recorded.

## DEFAULT BEHAVIOR AND TROUBLESHOOTING

Callbacks create a process span directly under extracted context, without an artificial completed
receive span. Actual Fetch calls have their own receive measurement. If asynchronous handler
errors are missing, set `WithErrorHandler`. If propagation is absent, check server header support
and whether both publisher and consumer use the configured propagator.

## ASYNC JETSTREAM PUBLISH

`PublishAsync` and `PublishMsgAsync` preserve the native signatures and instrument
enqueue through acknowledgement or failure. Use
`PublishMsgAsyncContext(ctx, msg, options...)` to attach an application parent.
Headers are copied before injection; application headers and payload are preserved.
The returned future forwards the native acknowledgement/error and `Msg()` result.

Cancellation after enqueue does not cancel broker delivery or end the span early.
Native retry, timeout, pending-count and completion APIs keep their original behavior.
Each pending future uses one completion goroutine. Read each returned future's
`Ok()` or `Err()` before flushing providers; the native `PublishAsyncComplete()`
signal alone does not guarantee the telemetry forwarding goroutine has finished.
On shutdown, `CleanupPublisher()` resolves native pending futures with errors;
then await those returned futures before flushing telemetry.
