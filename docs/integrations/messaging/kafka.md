# KAFKA

Choose one optional Go 1.26 module for your native client:

| Client | Module | API |
| --- | --- | --- |
| franz-go | `plugins/messaging/kafka/franzgo` | `New`, `Produce`, `ProduceSync`, `PollRecords`, `Process`, `ProcessBatch` |
| kafka-go | `plugins/messaging/kafka/kafkago` | `New`, `WriteMessages`, `FetchMessage`, `Process`, `ProcessBatch` |
| IBM Sarama | `plugins/messaging/kafka/sarama` | `New`, `SendMessage`, `SendMessages`, `Process`, `GroupHandler` |

```bash
go get github.com/muhammadluth/goslogx/plugins/messaging/kafka/franzgo
```

```go
observed, err := goslogxfranzgo.New(logger,
    goslogxfranzgo.WithConsumerGroup("orders-worker"),
    goslogxfranzgo.WithDestinationNormalizer(func(string) string { return "orders" }))
if err != nil { return err }
results := observed.ProduceSync(ctx, client, &kgo.Record{Topic: "orders", Value: payload})
if err := results.FirstErr(); err != nil { return err }
fetches := observed.PollRecords(ctx, client, 100)
if err := fetches.Err(); err != nil { return err }
return observed.ProcessBatch(ctx, fetches.Records(), func(ctx context.Context, record *kgo.Record) error {
    return processOrder(ctx, record.Value)
})
```

Producer helpers inject headers into copies and finish spans at native completion. Native retries and delivery metadata are retained. Helpers do not add their own retry loops or inspect record keys/values. Process extracts each record's context and creates receipt/process spans. Batch processing runs in order, attempts every record and joins handler errors; callers decide which records to retry and commit. Configure consumer groups on the native client and provide a stable group label with `WithConsumerGroup`. Poll/fetch spans cover the native polling call; per-record receipt counts come from Process.

kafka-go supports synchronous `Writer.WriteMessages`; native `Writer.Async` is rejected because its completion contract differs. Use FetchMessage followed by Process and your own CommitMessages; ReadMessage's automatic commits are not wrapped. IBM Sarama targets `github.com/IBM/sarama`, without obsolete Shopify dependencies. Its producer adapter supports SyncProducer, including per-message batch failures. `GroupHandler(setup, cleanup, handle)` delegates group lifecycle and passes the native session to your handler; call MarkMessage only according to your application's processing policy. AsyncProducer is outside this adapter.

Metrics include successful sent/received counts and producer/receive/process timing with bounded error dimensions. Kafka native client stats/hooks remain the source for broker lag, retries, wire bytes and partition metrics; these helpers do not poll brokers or fabricate unavailable values. No offsets or partitions become metric dimensions. See [privacy and auto-instrumentation coexistence](../../architecture/integration-policy.md).

## IMPLEMENTATION

This plugin uses client-specific franz-go, kafka-go, and Sarama wrappers over native client APIs. The supplied goslogx provider owns provider selection; the native client keeps connection, retry, transaction, acknowledgement, and shutdown behavior.

## AUTO-INSTRUMENTATION COEXISTENCE

Use either this plugin or another instrumentation layer for the same client operation. Enabling both can produce duplicate spans, duplicate metrics, incorrect parentage, and extra overhead.
