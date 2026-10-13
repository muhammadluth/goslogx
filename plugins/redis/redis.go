package redis

import (
	"context"
	"errors"
	"reflect"

	"github.com/muhammadluth/goslogx"
	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ScopeName identifies the integration without request-specific values.
const ScopeName = "github.com/muhammadluth/goslogx/plugins/redis"

type Option func(*config)

type config struct {
	closeChan chan struct{}
	onError   func(error)
}

// WithErrorHandler receives asynchronous initialization errors for future Ring nodes.
// It must be concurrency-safe. The plugin does not install a global error handler.
func WithErrorHandler(handler func(error)) Option { return func(c *config) { c.onError = handler } }

func WithCloseChan(ch chan struct{}) Option {
	return func(c *config) { c.closeChan = ch }
}

func Instrument(client goredis.UniversalClient, provider goslogx.TelemetryProvider, opts ...Option) error {
	if isNil(client) {
		return errors.New("redis instrumentation: nil client")
	}
	switch client.(type) {
	case *goredis.Client, *goredis.ClusterClient, *goredis.Ring:
	default:
		return errors.New("redis instrumentation: unsupported client")
	}
	var tp trace.TracerProvider = tracenoop.NewTracerProvider()
	var mp metric.MeterProvider = metricnoop.NewMeterProvider()
	if !isNil(provider) {
		if p := provider.TracerProvider(); !isNil(p) {
			tp = p
		}
		if p := provider.MeterProvider(); !isNil(p) {
			mp = p
		}
	}
	c := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	instrument := func(node *goredis.Client) {
		_ = redisotel.InstrumentTracing(node,
			redisotel.WithTracerProvider(privateTracerProvider{tp}),
			redisotel.WithDBStatement(false),
			redisotel.WithCallerEnabled(false),
		)
	}
	metricOptions := []redisotel.MetricsOption{
		redisotel.WithMeterProvider(scopedMeterProvider{MeterProvider: mp}), redisotel.WithPoolName("redis"),
		redisotel.WithCloseChan(c.closeChan), redisotel.WithSemConvCompliantMetrics(true),
	}
	if ring, ok := client.(*goredis.Ring); ok {
		// Ring constructs shards eagerly. Upstream OnNewNode sees only future nodes.
		initialize := func(node *goredis.Client) error {
			if err := redisotel.InstrumentMetrics(node, metricOptions...); err != nil {
				return err
			}
			instrument(node)
			return nil
		}
		if err := ring.ForEachShard(context.Background(), func(_ context.Context, node *goredis.Client) error { return initialize(node) }); err != nil {
			return err
		}
		ring.OnNewNode(func(node *goredis.Client) {
			if err := initialize(node); err != nil && c.onError != nil {
				c.onError(err)
			}
		})
		return nil
	}
	if err := redisotel.InstrumentMetrics(client, metricOptions...); err != nil {
		return err
	}
	switch client := client.(type) {
	case *goredis.Client:
		instrument(client)
	case *goredis.ClusterClient:
		client.OnNewNode(instrument)
	case *goredis.Ring:
		client.OnNewNode(instrument)
	}
	return nil
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// Keep the plugin's stable scope while retaining upstream instrument options.
type scopedMeterProvider struct{ metric.MeterProvider }

func (p scopedMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return p.MeterProvider.Meter(ScopeName, opts...)
}
