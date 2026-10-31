package franzgo

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/privacy"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// NativeInstrumentation uses kotel's hooks for producer completion, receive
// buffering and broker metrics. Install Hooks once on a new kgo.Client. Use
// native Produce/PollRecords with this API, not the legacy helper wrappers.
type NativeInstrumentation struct {
	tracer *kotel.Tracer
	hooks  []kgo.Hook
}

func NewNative(p goslogx.TelemetryProvider, opts ...Option) (*NativeInstrumentation, error) {
	cfg := configure(opts)
	t, err := telemetry.New(p, ScopeName, "kafka", true, func(c *telemetry.Config) { *c = cfg.Config })
	if err != nil {
		return nil, err
	}
	tp, mp, prop := telemetry.Resolve(p, t.Config)
	traceOpts := []kotel.TracerOpt{
		kotel.TracerProvider(telemetry.ConfiguredDBProvider(tp, ScopeName, t.Config, telemetry.DBPolicy{ExtraAttributes: cfg.keyAttributes})),
		kotel.TracerPropagator(prop), kotel.ClientID(t.Config.ClientID), kotel.ConsumerGroup(t.Config.Group),
		kotel.KeyFormatter(func(*kgo.Record) (string, error) { return "", errors.New("message keys are not captured") }),
	}
	if cfg.keyFormatter != nil {
		traceOpts = append(traceOpts, kotel.KeyFormatter(cfg.keyFormatter))
	}
	if t.Config.ConsumerLinks {
		traceOpts = append(traceOpts, kotel.LinkSpans())
	}
	tracer := kotel.NewTracer(traceOpts...)
	var initErr error
	mp = nativeMeterProvider{MeterProvider: mp, t: t, initErr: &initErr}
	meterOptions := []kotel.MeterOpt{kotel.MeterProvider(mp)}
	if cfg.mergedConnects {
		meterOptions = append(meterOptions, kotel.WithMergedConnectsMeter())
	}
	meter := kotel.NewMeter(meterOptions...)
	if initErr != nil {
		return nil, initErr
	}
	return &NativeInstrumentation{tracer: tracer, hooks: kotel.NewKotel(kotel.WithTracer(tracer), kotel.WithMeter(meter)).Hooks()}, nil
}

func (i *NativeInstrumentation) Hooks() []kgo.Hook { return append([]kgo.Hook(nil), i.hooks...) }

// Process preserves kotel's receive context or configured links. The application
// retains responsibility for committing offsets and ending its processing work.
func (i *NativeInstrumentation) Process(ctx context.Context, record *kgo.Record, handler func(context.Context, *kgo.Record) error) (err error) {
	if record == nil || handler == nil {
		return errors.New("franzgo instrumentation: nil record or handler")
	}
	copy := *record
	if copy.Context == nil {
		copy.Context = ctx
	}
	child, span := i.tracer.WithProcessSpan(&copy)
	defer func() {
		if r := recover(); r != nil {
			privacy.RecordError(span, errors.New("handler panic"))
			span.SetStatus(codes.Error, "handler panic")
			span.End()
			panic(r)
		}
		if err != nil {
			privacy.RecordError(span, err)
			span.SetStatus(codes.Error, "operation failed")
		}
		span.End()
	}()
	return handler(child, record)
}

type nativeMeterProvider struct {
	metric.MeterProvider
	t       *telemetry.Instrumentation
	initErr *error
}

func (p nativeMeterProvider) Meter(_ string, opts ...metric.MeterOption) metric.Meter {
	return nativeMeter{Meter: p.MeterProvider.Meter(ScopeName, opts...), t: p.t, initErr: p.initErr}
}

type nativeMeter struct {
	metric.Meter
	t       *telemetry.Instrumentation
	initErr *error
}

func (m nativeMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	c, err := m.Meter.Int64Counter(name, opts...)
	if err != nil {
		*m.initErr = errors.Join(*m.initErr, err)
		return c, err
	}
	return nativeCounter{Int64Counter: c, t: m.t}, nil
}

type nativeCounter struct {
	metric.Int64Counter
	t *telemetry.Instrumentation
}

func (c nativeCounter) Add(ctx context.Context, value int64, opts ...metric.AddOption) {
	set := metric.NewAddConfig(opts).Attributes()
	attrs := append([]attribute.KeyValue(nil), c.t.Config.MetricAttributes...)
	for _, a := range set.ToSlice() {
		switch a.Key {
		case "node_id", "outcome":
			attrs = append(attrs, a)
		case "topic":
			if topic := c.t.Destination(a.Value.AsString()); topic != "" {
				attrs = append(attrs, attribute.String("topic", topic))
			}
		}
	}
	c.Int64Counter.Add(ctx, value, metric.WithAttributes(attrs...))
}

// Compile-time assertions keep hook availability tied to the pinned upstream.
var _ kgo.HookBrokerWrite = (*kotel.Meter)(nil)
var _ kgo.HookProduceRecordUnbuffered = (*kotel.Tracer)(nil)
