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

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

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
	cfg := telemetry.Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	tp, mp, _ := telemetry.Resolve(p, cfg)
	meter := mp.Meter(ScopeName)
	duration, err := meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return nil, err
	}
	native := otelmongo.NewMonitor(otelmongo.WithCommandAttributeDisabled(true), otelmongo.WithTracerProvider(telemetry.SafeDBProvider(tp, ScopeName)), otelmongo.WithMeterProvider(mongoMeterProvider{MeterProvider: mp, histogram: privateHistogram{Float64Histogram: duration}}), otelmongo.WithSpanNameFormatter(func(e *event.CommandStartedEvent) string { return telemetry.DBOperation(e.CommandName) }))
	// Do not forward command documents, filters, reply documents or raw failure payloads.
	command := &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			copy := *e
			copy.Command = nil
			copy.DatabaseName = cfg.Namespace
			copy.CommandName = telemetry.DBOperation(e.CommandName)
			native.Started(ctx, &copy)
		},
		Succeeded: func(ctx context.Context, e *event.CommandSucceededEvent) {
			copy := *e
			copy.Reply = nil
			copy.DatabaseName = cfg.Namespace
			copy.CommandName = telemetry.DBOperation(e.CommandName)
			native.Succeeded(ctx, &copy)
		},
		Failed: func(ctx context.Context, e *event.CommandFailedEvent) {
			copy := *e
			copy.DatabaseName = cfg.Namespace
			copy.CommandName = telemetry.DBOperation(e.CommandName)
			copy.Failure = errors.New("database_error")
			native.Failed(ctx, &copy)
		},
	}
	connections, err := meter.Int64UpDownCounter("db.client.connection.count", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	attrs := []attribute.KeyValue{attribute.String("db.system.name", "mongodb")}
	if cfg.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", cfg.Namespace))
	}
	type key struct {
		address string
		id      int64
	}
	states := make(map[key]string)
	var mu sync.Mutex
	pool := &event.PoolMonitor{Event: func(e *event.PoolEvent) {
		mu.Lock()
		defer mu.Unlock()
		k := key{e.Address, e.ConnectionID}
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
				if k.address == e.Address {
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
	histogram metric.Float64Histogram
}

func (p mongoMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return mongoMeter{Meter: p.MeterProvider.Meter(ScopeName, opts...), histogram: p.histogram}
}

type mongoMeter struct {
	metric.Meter
	histogram metric.Float64Histogram
}

func (m mongoMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return m.histogram, nil
}

type privateHistogram struct{ metric.Float64Histogram }

func (h privateHistogram) Record(ctx context.Context, value float64, opts ...metric.RecordOption) {
	cfg := metric.NewRecordConfig(opts)
	set := cfg.Attributes()
	attrs := make([]attribute.KeyValue, 0, 4)
	for _, a := range set.ToSlice() {
		switch a.Key {
		case "db.system.name", "db.namespace":
			attrs = append(attrs, a)
		case "db.operation.name":
			attrs = append(attrs, attribute.String("db.operation.name", telemetry.DBOperation(a.Value.AsString())))
		case "error.type":
			attrs = append(attrs, attribute.String("error.type", "database_error"))
		}
	}
	h.Float64Histogram.Record(ctx, value, metric.WithAttributes(attrs...))
}
