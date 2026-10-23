package redis

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// ScopeName identifies the integration without request-specific values.
const ScopeName = "github.com/muhammadluth/goslogx/plugins/cache/redis"

type Option func(*config)

type config struct {
	tracing                 bool
	metrics                 bool
	dbSystem                string
	attributes              []attribute.KeyValue
	poolName                string
	dbStatement             bool
	callerEnabled           bool
	commandFilter           func(goredis.Cmder) bool
	commandsFilter          func([]goredis.Cmder) bool
	dialFilter              bool
	closeChan               chan struct{}
	semConvCompliantMetrics bool
	onError                 func(error)
}

func defaultConfig() config {
	return config{
		tracing:                 true,
		metrics:                 true,
		dbSystem:                "redis",
		poolName:                "redis",
		semConvCompliantMetrics: true,
	}
}

// WithTracing enables or disables Redis tracing. It is enabled by default.
func WithTracing(enabled bool) Option { return func(c *config) { c.tracing = enabled } }

// WithMetrics enables or disables Redis pool metrics. It is enabled by default.
func WithMetrics(enabled bool) Option { return func(c *config) { c.metrics = enabled } }

// WithDBSystem sets the bounded database system name on Redis telemetry.
func WithDBSystem(system string) Option { return func(c *config) { c.dbSystem = system } }

// WithAttributes adds validated static attributes to Redis spans and metrics.
func WithAttributes(attrs ...attribute.KeyValue) Option {
	return func(c *config) { c.attributes = append(c.attributes, attrs...) }
}

// WithPoolName sets the pool.name metric attribute. Keep it bounded.
func WithPoolName(name string) Option { return func(c *config) { c.poolName = name } }

// WithDBStatement enables raw Redis command capture. It can expose keys and
// values and is disabled by default.
func WithDBStatement(enabled bool) Option { return func(c *config) { c.dbStatement = enabled } }

// WithCallerEnabled enables runtime caller discovery. It is disabled by
// default because it adds runtime overhead and source location attributes.
func WithCallerEnabled(enabled bool) Option { return func(c *config) { c.callerEnabled = enabled } }

// WithCommandFilter skips tracing when filter returns true. When unset,
// redisotel's default filter suppresses AUTH and HELLO AUTH commands.
func WithCommandFilter(filter func(goredis.Cmder) bool) Option {
	return func(c *config) { c.commandFilter = filter }
}

// WithCommandsFilter skips pipeline tracing when filter returns true. When
// unset, redisotel suppresses a pipeline containing a sensitive auth command.
func WithCommandsFilter(filter func([]goredis.Cmder) bool) Option {
	return func(c *config) { c.commandsFilter = filter }
}

// WithDialFilter follows redisotel semantics: true filters dial spans.
func WithDialFilter(filtered bool) Option { return func(c *config) { c.dialFilter = filtered } }

// WithSemConvCompliantMetrics selects monotonic cumulative pool counters.
func WithSemConvCompliantMetrics(enabled bool) Option {
	return func(c *config) { c.semConvCompliantMetrics = enabled }
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
	c := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	if strings.TrimSpace(c.dbSystem) == "" {
		c.dbSystem = "redis"
	}
	if strings.TrimSpace(c.poolName) == "" {
		c.poolName = "redis"
	}
	if !c.tracing && !c.metrics {
		return nil
	}
	c.attributes = safeConfiguredAttributes(c.attributes)
	common := []redisotel.Option{
		redisotel.WithDBSystem(c.dbSystem),
		redisotel.WithAttributes(c.attributes...),
		redisotel.WithPoolName(c.poolName),
	}
	traceOptions := []redisotel.TracingOption{
		redisotel.WithTracerProvider(privateTracerProvider{TracerProvider: tp, policy: privacyPolicy{
			statement: c.dbStatement,
			caller:    c.callerEnabled,
			dbSystem:  c.dbSystem,
			custom:    attributeKeys(c.attributes),
		}}),
		redisotel.WithDBStatement(c.dbStatement),
		redisotel.WithCallerEnabled(c.callerEnabled),
		redisotel.WithDialFilter(c.dialFilter),
	}
	for _, opt := range common {
		traceOptions = append(traceOptions, opt)
	}
	if c.commandFilter != nil {
		traceOptions = append(traceOptions, redisotel.WithCommandFilter(func(cmd goredis.Cmder) bool {
			return redisotel.DefaultCommandFilter(cmd) || c.commandFilter(cmd)
		}))
	}
	if c.commandsFilter != nil {
		traceOptions = append(traceOptions, redisotel.WithCommandsFilter(func(cmds []goredis.Cmder) bool {
			for _, cmd := range cmds {
				if redisotel.DefaultCommandFilter(cmd) {
					return true
				}
			}
			return c.commandsFilter(cmds)
		}))
	}
	instrument := func(node *goredis.Client) {
		if c.tracing {
			_ = redisotel.InstrumentTracing(node, traceOptions...)
		}
	}
	metricOptions := []redisotel.MetricsOption{
		redisotel.WithMeterProvider(scopedMeterProvider{MeterProvider: mp}),
		redisotel.WithCloseChan(c.closeChan),
		redisotel.WithSemConvCompliantMetrics(c.semConvCompliantMetrics),
	}
	for _, opt := range common {
		metricOptions = append(metricOptions, opt)
	}
	if ring, ok := client.(*goredis.Ring); ok {
		// Ring constructs shards eagerly. Upstream OnNewNode sees only future nodes.
		initialize := func(node *goredis.Client) error {
			if c.metrics {
				if err := redisotel.InstrumentMetrics(node, metricOptions...); err != nil {
					return err
				}
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
	if c.metrics {
		if err := redisotel.InstrumentMetrics(client, metricOptions...); err != nil {
			return err
		}
	}
	if !c.tracing {
		return nil
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
