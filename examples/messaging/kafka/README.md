# Kafka example

Wrap a franz-go client operation while keeping native delivery callbacks:

```go
instrumentation, err := goslogxfranzgo.New(logger)
if err != nil { return err }
results := instrumentation.ProduceSync(ctx, client, &kgo.Record{Topic: "orders", Value: payload})
return results.FirstErr()
```

The [Kafka integration guide](../../../docs/integrations/messaging/kafka.md) also covers kafka-go and IBM Sarama.
