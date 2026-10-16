package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	metrics "github.com/muhammadluth/goslogx/internal/telemetry/metrics"
	propagationimpl "github.com/muhammadluth/goslogx/internal/telemetry/propagation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// ErrTracingShutdown indicates that the logger-owned tracing provider has
// already begun shutting down.
var ErrTracingShutdown = errors.New("goslogx: tracing provider is shut down")

// ErrMetricsShutdown indicates that managed metrics have stopped.
var ErrMetricsShutdown = errors.New("goslogx: metrics provider is shut down")
var telemetryGlobalsMu sync.Mutex

// A pointer wrapper lets shutdown recognize its own global registration even
// when another application installs a non-comparable composite propagator.
type managedPropagator struct{ propagation.TextMapPropagator }
type State struct {
	TracerProvider                            *sdktrace.TracerProvider
	MeterProvider                             *sdkmetric.MeterProvider
	Recorder                                  *metrics.Recorder
	Closed                                    atomic.Bool
	Gate                                      chan struct{}
	shutdownOnce                              sync.Once
	shutdownErr                               error
	otelHandler                               *ErrorHandler
	previousOTelHandler                       otel.ErrorHandler
	registeredGlobal, registeredGlobalMetrics bool
	previousProvider                          trace.TracerProvider
	previousMeterProvider                     metric.MeterProvider
	previousPropagator, installedPropagator   propagation.TextMapPropagator
}

func New(tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider, recorder *metrics.Recorder) *State {
	return &State{TracerProvider: tp, MeterProvider: mp, Recorder: recorder, Gate: make(chan struct{}, 1)}
}

func (l *State) Register(logger *zap.Logger, registerTracing, registerMetrics bool, traceInterval, metricInterval time.Duration) {
	telemetryGlobalsMu.Lock()
	defer telemetryGlobalsMu.Unlock()
	if l.MeterProvider != nil && registerMetrics {
		l.previousMeterProvider = otel.GetMeterProvider()
		otel.SetMeterProvider(l.MeterProvider)
		l.registeredGlobalMetrics = true
	}
	if l.TracerProvider != nil && registerTracing {
		l.previousProvider = otel.GetTracerProvider()
		l.previousPropagator = otel.GetTextMapPropagator()
		otel.SetTracerProvider(l.TracerProvider)
		l.installedPropagator = &managedPropagator{TextMapPropagator: propagationimpl.Legacy()}
		otel.SetTextMapPropagator(l.installedPropagator)
		l.registeredGlobal = true
	}
	if l.registeredGlobal || l.registeredGlobalMetrics {
		l.previousOTelHandler = otel.GetErrorHandler()
		interval := traceInterval
		if !l.registeredGlobal {
			interval = metricInterval
		}
		l.otelHandler = NewErrorHandler(logger, interval, l.Recorder)
		otel.SetErrorHandler(l.otelHandler)
	}

}

// ForceFlush exports pending telemetry from logger-owned providers.
func (l *State) ForceFlush(ctx context.Context) (err error) {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer func() {
		l.Recorder.RecordForceFlush(ctx, time.Since(started), err)
	}()

	if l.TracerProvider == nil && l.MeterProvider == nil {
		return nil
	}
	select {
	case l.Gate <- struct{}{}:
		defer func() { <-l.Gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if l.Closed.Load() {
		if l.TracerProvider != nil {
			return ErrTracingShutdown
		}
		return ErrMetricsShutdown
	}
	if l.MeterProvider != nil {
		err = l.MeterProvider.ForceFlush(ctx)
	}
	if l.TracerProvider != nil {
		err = errors.Join(err, l.TracerProvider.ForceFlush(ctx))
	}
	return err
}

// Shutdown idempotently stops owned metric and trace providers. External
// providers remain application-owned. Stop application work before calling.
func (l *State) Shutdown(ctx context.Context) (err error) {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	defer func() {
		l.Recorder.RecordShutdown(ctx, time.Since(started), err)
	}()

	if l.TracerProvider == nil && l.MeterProvider == nil {
		return nil
	}
	select {
	case l.Gate <- struct{}{}:
		defer func() { <-l.Gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	l.shutdownOnce.Do(func() {
		l.Closed.Store(true)
		if l.MeterProvider != nil {
			l.shutdownErr = l.MeterProvider.Shutdown(ctx)
		}
		if l.TracerProvider != nil {
			l.shutdownErr = errors.Join(l.shutdownErr, l.TracerProvider.Shutdown(ctx))
		}
		telemetryGlobalsMu.Lock()
		defer telemetryGlobalsMu.Unlock()
		if l.registeredGlobalMetrics && otel.GetMeterProvider() == l.MeterProvider {
			otel.SetMeterProvider(l.previousMeterProvider)
		}
		if l.registeredGlobal && otel.GetTracerProvider() == l.TracerProvider {
			otel.SetTracerProvider(l.previousProvider)
		}
		if l.registeredGlobal && otel.GetTextMapPropagator() == l.installedPropagator {
			otel.SetTextMapPropagator(l.previousPropagator)
		}
		if current, ok := otel.GetErrorHandler().(*ErrorHandler); ok && current == l.otelHandler {
			otel.SetErrorHandler(l.previousOTelHandler)
		}
	})
	return l.shutdownErr
}
