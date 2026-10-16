# Redis example

The [full observability example](../../full-observability/README.md) records a Redis operation in the same trace as HTTP, Bun and NATS. Focused setup:

```go
stop := make(chan struct{})
defer close(stop)
if err := goslogxredis.Instrument(client, logger, goslogxredis.WithCloseChan(stop)); err != nil { return err }
return client.Set(ctx, "order:state", "created", 0).Err()
```

See the [Redis integration guide](../../../docs/integrations/cache/redis.md).
