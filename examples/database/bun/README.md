# Bun example

The [full observability example](../../full-observability/README.md) instruments a real Bun database and executes a query under an HTTP request trace. For the focused setup:

```go
if err := goslogxbun.Instrument(db, logger, goslogxbun.WithDBName("orders")); err != nil { return err }
_, err := db.ExecContext(ctx, "INSERT INTO orders (id) VALUES (?)", id)
return err
```

See the [Bun integration guide](../../../docs/integrations/database/bun.md).
