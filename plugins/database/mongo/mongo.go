// Package mongo configures the OpenTelemetry contrib monitor for MongoDB driver v2.
package mongo

import (
	"context"
	"errors"
	"sync"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/mongo"

type Monitors struct {
	Command *event.CommandMonitor
	Pool    *event.PoolMonitor
}

// Instrument configures options before mongo.Connect and rejects existing monitors.
func Instrument(config *options.ClientOptions, p goslogx.TelemetryProvider, opts ...Option) error {
	if config == nil {
		return errors.New("mongo instrumentation: nil client options")
	}
	if config.Monitor != nil || config.PoolMonitor != nil {
		return errors.New("mongo instrumentation: monitor already configured; choose one instrumentation layer")
	}
	monitors, err := NewMonitors(p, opts...)
	if err != nil {
		return err
	}
	config.SetMonitor(monitors.Command)
	config.SetPoolMonitor(monitors.Pool)
	return nil
}
func NewMonitors(p goslogx.TelemetryProvider, opts ...Option) (*Monitors, error) {
	cfg := config{}
	telemetry.ApplyOptions(&cfg.Config, &cfg, opts...)
	tp, mp, _ := telemetry.Resolve(p, cfg.Config)
	meter := mp.Meter(ScopeName)
	if cfg.StatementCapture && cfg.StatementSanitizer == nil && !cfg.RawStatement {
		return nil, errors.New("mongo: command capture requires a sanitizer")
	}
	var initErr error
	optsNative := []otelmongo.Option{
		otelmongo.WithCommandAttributeDisabled(!cfg.StatementCapture),
		otelmongo.WithTracerProvider(telemetry.ConfiguredDBProvider(tp, ScopeName, cfg.Config)),
		otelmongo.WithMeterProvider(mongoMeterProvider{MeterProvider: mp, config: cfg.Config, initErr: &initErr}),
	}
	if cfg.formatter != nil {
		optsNative = append(optsNative, otelmongo.WithSpanNameFormatter(cfg.formatter))
	}
	native := otelmongo.NewMonitor(optsNative...)
	if initErr != nil {
		return nil, initErr
	}
	// The native monitor reads command metadata and handles request matching.
	// Raw BSON capture remains disabled unless explicitly configured above.
	command := native
	if cfg.Namespace != "" {
		command = &event.CommandMonitor{
			Started: func(ctx context.Context, e *event.CommandStartedEvent) {
				copy := *e
				copy.DatabaseName = cfg.Namespace
				native.Started(ctx, &copy)
			},
			Succeeded: func(ctx context.Context, e *event.CommandSucceededEvent) {
				copy := *e
				copy.DatabaseName = cfg.Namespace
				native.Succeeded(ctx, &copy)
			},
			Failed: func(ctx context.Context, e *event.CommandFailedEvent) {
				copy := *e
				copy.DatabaseName = cfg.Namespace
				native.Failed(ctx, &copy)
			},
		}
	}
	connections, err := meter.Int64UpDownCounter("db.client.connection.count", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	attrs := append([]attribute.KeyValue{attribute.String("db.system.name", "mongodb")}, cfg.MetricAttributes...)
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
	}
	states := make(map[ConnectionKey]string)
	var mu sync.Mutex
	pool := &event.PoolMonitor{Event: func(e *event.PoolEvent) {
		mu.Lock()
		defer mu.Unlock()
		k := ConnectionKey{Address: e.Address, ID: e.ConnectionID}
		change := func(state string, delta int64) {
			a := append(append([]attribute.KeyValue(nil), attrs...), attribute.String("db.client.connection.state", state))
			connections.Add(context.Background(), delta, metric.WithAttributes(a...))
		}
		previous := states[k]
		next := ""
		switch e.Type {
		case event.ConnectionReady:
			next = "idle"
		case event.ConnectionCheckedOut:
			next = "used"
		case event.ConnectionCheckedIn:
			next = "idle"
		case event.ConnectionClosed:
		case event.ConnectionPoolClosed:
			for k, state := range states {
				if k.Address == e.Address {
					change(state, -1)
					delete(states, k)
				}
			}
			return
		default:
			return
		}
		if next == previous {
			return
		}
		if previous != "" {
			change(previous, -1)
		}
		if next != "" {
			change(next, 1)
			states[k] = next
		} else {
			delete(states, k)
		}
	}}
	return &Monitors{command, pool}, nil
}

type mongoMeterProvider struct {
	metric.MeterProvider
	config  telemetry.Config
	initErr *error
}

func (p mongoMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return mongoMeter{Meter: p.MeterProvider.Meter(ScopeName, opts...), config: p.config, initErr: p.initErr}
}

type mongoMeter struct {
	metric.Meter
	config  telemetry.Config
	initErr *error
}

func (m mongoMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	h, err := m.Meter.Float64Histogram(name, opts...)
	if err != nil {
		*m.initErr = errors.Join(*m.initErr, err)
		return h, err
	}
	return privateHistogram{Float64Histogram: h, config: m.config}, nil
}

type privateHistogram struct {
	metric.Float64Histogram
	config telemetry.Config
}

func (h privateHistogram) Record(ctx context.Context, value float64, opts ...metric.RecordOption) {
	set := metric.NewRecordConfig(opts).Attributes()
	attrs := append([]attribute.KeyValue(nil), h.config.MetricAttributes...)
	for _, a := range set.ToSlice() {
		switch a.Key {
		case "db.system.name", "db.namespace", "db.operation.name", "network.peer.address", "network.peer.port", "network.transport", "error.type", "db.response.status_code":
			attrs = append(attrs, a)
		}
	}
	h.Float64Histogram.Record(ctx, value, metric.WithAttributes(attrs...))
}
