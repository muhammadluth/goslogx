# MongoDB example

Install monitors before connecting with the MongoDB v2 driver:

```go
opts := options.Client().ApplyURI(uri)
if err := goslogxmongo.Instrument(opts, logger, goslogxmongo.WithDBName("orders")); err != nil { return err }
client, err := mongo.Connect(opts)
if err != nil { return err }
defer client.Disconnect(ctx)
return client.Ping(ctx, nil)
```

See the [MongoDB integration guide](../../../docs/integrations/database/mongo.md).
