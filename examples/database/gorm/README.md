# GORM example

Register the plugin before queries and pass the request context:

```go
if err := goslogxgorm.Instrument(db, logger, goslogxgorm.WithDBName("orders")); err != nil { return err }
return db.WithContext(ctx).Create(&order).Error
```

See the [GORM integration guide](../../../docs/integrations/database/gorm.md) for transaction spans and SQL-layer coexistence.
