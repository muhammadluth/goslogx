package pg

import (
	"context"
	"github.com/go-pg/pg/v10"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Pool observations are collected by the supplied provider, with no plugin goroutine.
func observePool(meter metric.Meter, db *pg.DB, attributes []attribute.KeyValue) error {
	_, err := meter.Int64ObservableUpDownCounter("db.client.connection.count",
		metric.WithUnit("{connection}"),
		metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
			stats := db.PoolStats()
			used := append(append([]attribute.KeyValue(nil), attributes...), attribute.String("db.client.connection.state", "used"))
			idle := append(append([]attribute.KeyValue(nil), attributes...), attribute.String("db.client.connection.state", "idle"))
			observer.Observe(int64(stats.TotalConns)-int64(stats.IdleConns), metric.WithAttributes(used...))
			observer.Observe(int64(stats.IdleConns), metric.WithAttributes(idle...))
			return nil
		}))
	return err
}
