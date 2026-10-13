# go-redis v9

Import `github.com/muhammadluth/goslogx/plugins/redis` as `goslogxredis`:

```go
closeMetrics := make(chan struct{})
if err := goslogxredis.Instrument(client, logger,
    goslogxredis.WithCloseChan(closeMetrics),
); err != nil { return err }
value, err := client.Get(ctx, "inventory").Result()
// During application shutdown:
_ = client.Close()
close(closeMetrics)
```

The official redisotel v9.23.0 hook supports Client, ClusterClient, Ring and UniversalClient values holding these concrete clients. Register once before concurrent use, and before the first operation for ClusterClient. The adapter instruments existing Ring shards plus future nodes. `WithErrorHandler` receives asynchronous initialization errors from future Ring nodes; supply a concurrency-safe handler if your meter can reject instrument creation. Providers stay explicit, including tracing-only/metrics-only and no-op modes. Redis commands continue the active context; pipelines create a bounded pipeline span. `redis.Nil` represents a missing key and does not set error status.

The private tracer boundary uses fixed command/pipeline/dial names, disables statements/caller capture, and omits keys, values, arguments, server addresses and raw error events/descriptions. Metrics use a fixed `pool.name=redis` and upstream fixed status/state dimensions. The integration does not expose statement or key capture.

Upstream instruments retain names such as `db.client.connections.use_time` and `.create_time` (milliseconds), `.usage`, `.max`, idle limits, waits/wait duration, timeouts, hits and misses. Failed operations use the bounded status label in duration measurements. `WithSemConvCompliantMetrics(true)` selects monotonic instruments for cumulative pool totals; metric names remain upstream legacy names.

CloseChan releases upstream metric callback registrations. Without it, callbacks live until the provider shuts down. Go 1.26 is required by the pinned upstream dependencies.
