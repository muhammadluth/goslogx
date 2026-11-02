# KAFKA

## INSTALLATION

Choose the module for your native client. These modules require Go 1.26.

| Client | Canonical module | Instrumentation |
| --- | --- | --- |
| franz-go | `github.com/muhammadluth/goslogx/plugins/messaging/kafka/franzgo` | Native kotel hooks or compatibility helpers |
| kafka-go | `github.com/muhammadluth/goslogx/plugins/messaging/kafka/kafkago` | Native writer completion and reader wrappers |
| IBM Sarama | `github.com/muhammadluth/goslogx/plugins/messaging/kafka/sarama` | Sync producer, async producer and consumer group wrappers |

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/kafka/franzgo
```

## FRANZ-GO QUICK START

Use [kotel v1.7.1](https://github.com/twmb/franz-go/tree/plugin/kotel/v1.7.1/plugin/kotel)
through `NewNative` for native producer/consumer lifecycle hooks and broker metrics:

```go
observed, err := goslogxfranzgo.NewNative(logger,
    goslogxfranzgo.WithConsumerGroup("orders-worker"),
    goslogxfranzgo.WithDestinationNormalizer(func(string) string { return "orders" }),
)
if err != nil { return err }
client, err := kgo.NewClient(
    kgo.SeedBrokers(brokers...),
    kgo.WithHooks(observed.Hooks()...),
)
if err != nil { return err }
defer client.Close()

results := client.ProduceSync(ctx, &kgo.Record{Topic: "orders", Value: payload})
if err := results.FirstErr(); err != nil { return err }
for _, record := range client.PollRecords(ctx, 100).Records() {
    if err := observed.Process(ctx, record, processOrder); err != nil { return err }
}
```

Use native `Produce`, `ProduceSync` and `PollRecords` with these hooks. kotel owns publish,
receive and process names, timing and context. `WithConsumerLinks(true)` selects kotel's
span-link mode. Message keys stay private. Native span names may include topic names;
metric topics require a destination normalizer.

The older `New` helpers remain available: `Produce`, `ProduceSync`, `PollRecords`, `Process`
and `ProcessBatch`. Use one path per client. Combining native hooks and helper wrappers
instruments the same operation twice.

## KAFKA-GO ASYNCHRONOUS WRITERS

For a synchronous writer, use `New` and `WriteMessages`. For an asynchronous writer, wrap
it before its first write:

```go
writer := &kafka.Writer{
    Addr: kafka.TCP(brokers...), Topic: "orders", Async: true,
}
observed, err := goslogxkafkago.WrapAsyncWriter(writer, logger)
if err != nil { return err }
defer observed.Close()
err = observed.WriteMessages(ctx, kafka.Message{Value: payload})
```

The wrapper leaves `Async` enabled and ends producer spans in the native `Completion`
callback. Existing callbacks receive the original `WriterData`. A nil return from an
asynchronous write means accepted for delivery; check completion for delivery errors.
The synchronous helper rejects an unwrapped asynchronous `*kafka.Writer`.

Use `FetchMessage`, then `Process`, followed by your commit policy. The adapter does not
wrap `ReadMessage`'s automatic commit behavior.

## SARAMA ASYNCHRONOUS PRODUCERS

Configure the native completion channels before construction:

```go
config := sarama.NewConfig()
config.Producer.Return.Successes = true
config.Producer.Return.Errors = true
producer, err := sarama.NewAsyncProducer(brokers, config)
if err != nil { return err }
observed, err := goslogxsarama.WrapAsyncProducer(producer, config, logger)
if err != nil { _ = producer.Close(); return err }
// Start draining observed.Successes() and observed.Errors() before sending.
err = observed.Send(ctx, &sarama.ProducerMessage{Topic: "orders", Value: payload})
```

Pass the same config used to create the producer. `Send` supplies a parent context; native
`Input()` also works and creates a root producer span. Drain both output channels, as Sarama
requires. They return the original message pointers, application `Metadata`, offsets and
partitions. The wrapper keeps injected headers on its transport copy.

`AsyncClose` starts shutdown; continue draining completions until both channels close.
`Close` drains the channels and returns collected native delivery errors. Transaction methods
remain available through the embedded producer. Use only the wrapper after construction.

The sync APIs `SendMessage` and `SendMessages` preserve native batch errors and assignments.
`GroupHandler(setup, cleanup, handle)` passes the native session to your processing callback;
your application decides when to call `MarkMessage`.

## METRICS AND PROPAGATION

The native franz-go path retains kotel broker instruments, including
`messaging.kafka.write_bytes` and `messaging.kafka.read_bytes` with unit `by`, and their native
counter semantics. Destination normalization bounds topic dimensions. It does not turn
payloads, keys or offsets into metric labels.

Compatibility helpers and kafka-go/Sarama wrappers use `messaging.client.operation.duration`,
`messaging.process.duration`, `messaging.client.sent.messages` and
`messaging.client.consumed.messages`. Processing counts include failed handler attempts.
Each Process call creates one processing span under extracted context; it adds no synthetic
instantaneous receive span. Native polling spans cover the actual poll/fetch call.

## OPTIONS, PRIVACY AND LIFECYCLE

Use `WithTracing`, `WithMetrics`, `WithPropagator`, `WithTraceAttributes`, `WithMetricAttributes`,
`WithClientID`, `WithConsumerGroup` and `WithDestinationNormalizer` as applicable to each API.
Configure stable labels. The normalizer allows at most 64 distinct destinations per instance.
Baggage propagates only through the configured propagator; message values and raw errors stay
out of exported telemetry.

Close clients before flushing and shutting down providers. Keep retries, commits and business
processing decisions in your application. See the [integration policy](../../architecture/integration-policy.md).

## TROUBLESHOOTING

A producer span that remains open indicates an outstanding native completion. Drain Sarama
completion channels and check kafka-go Completion errors. Do not install another OTel layer
on the same producer. Native hooks and wrappers cannot detect instrumentation installed by
external agents.

## NATIVE OPTIONS

For `franzgo.NewNative`, `WithMergedConnectsMetrics(true)` selects kotel's
`messaging.kafka.connects.count` with an `outcome` dimension instead of a separate connection-error
counter. `WithRecordKeyFormatter` can return a sanitized key or an error to omit it. Formatted keys
are span-only, limited to 4096 bytes, and omitted by default. These two options require `NewNative`;
the legacy `New` helpers reject them rather than silently ignoring them.

Sarama transaction methods are serialized with forwarding of accepted messages, so CommitTxn
cannot overtake a prior `Input` or `Send`. Continue draining completion channels during transactions
and asynchronous shutdown, as required by the native producer.
