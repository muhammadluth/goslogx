# pgx example

Configure the native pool before opening it:

```go
config, err := pgxpool.ParseConfig(dsn)
if err != nil { return err }
if err := goslogxpgx.InstrumentConfig(config, logger, goslogxpgx.WithDBName("orders")); err != nil { return err }
pool, err := pgxpool.NewWithConfig(ctx, config)
if err != nil { return err }
defer pool.Close()
return pool.Ping(ctx)
```

See the [pgx integration guide](../../../docs/integrations/database/pgx.md) for imports and pool metrics.
