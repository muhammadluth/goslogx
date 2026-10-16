# RabbitMQ example

Wrap an existing AMQP channel and keep broker acknowledgement policy in application code:

```go
client, err := goslogxrabbitmq.WrapChannel(channel, logger)
if err != nil { return err }
return client.Publish(ctx, "events", "orders.created", false, false, amqp.Publishing{Body: payload})
```

See the [RabbitMQ integration guide](../../../docs/integrations/messaging/rabbitmq.md) for consuming and processing deliveries.
