package pgx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/exaring/otelpgx"
	native "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestNativeQueryParityAndRootFallback(t *testing.T) {
	for _, parented := range []bool{true, false} {
		for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
			t.Run(fmt.Sprint(parented, signals), func(t *testing.T) {
				p := testkit.New(t)
				h, err := newTracer(p, []Option{nil, WithTracing(signals[0]), WithMetrics(signals[1]), WithDBName("orders"), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")), WithPropagator(propagation.TraceContext{})})
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				var parent trace.Span
				if parented {
					ctx, parent = p.TP.Tracer("app").Start(ctx, "parent")
					defer parent.End()
				}
				query := h.TraceQueryStart(ctx, nil, native.TraceQueryStartData{SQL: "SELECT 'SECRET'", Args: []any{"SECRET"}})
				h.TraceQueryEnd(query, nil, native.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 2")})
				spans := p.Recorder.Ended()
				if (len(spans) == 1) != signals[0] {
					t.Fatal("tracing toggle", len(spans))
				}
				if signals[0] {
					span := spans[0]
					if span.Name() != "SELECT" || span.SpanKind() != trace.SpanKindClient {
						t.Fatal("native operation lost")
					}
					if parented && span.Parent().SpanID() != parent.SpanContext().SpanID() {
						t.Fatal("parent changed")
					}
					if !strings.Contains(fmt.Sprint(span.Attributes()), "pgx.rows_affected") {
						t.Fatal("native rows lost")
					}
				}
				if (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
					t.Fatal("metrics toggle")
				}
				p.AssertPrivate(t, "SECRET")
			})
		}
	}
	// Compare safe telemetry with the pinned implementation under a recording parent.
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	direct := otelpgx.NewTracer(otelpgx.WithTracerProvider(p.TP), otelpgx.WithMeterProvider(p.MP))
	adapter, _ := newTracer(p, nil)
	for _, h := range []native.QueryTracer{direct, adapter} {
		child := h.TraceQueryStart(ctx, nil, native.TraceQueryStartData{SQL: "SELECT 1"})
		h.TraceQueryEnd(child, nil, native.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})
	}
	a, b := p.Recorder.Ended()[0], p.Recorder.Ended()[1]
	if a.Name() != b.Name() || a.SpanKind() != b.SpanKind() || !a.Parent().Equal(b.Parent()) || a.Status() != b.Status() {
		t.Fatal("native parity")
	}
}

func TestAllNativeLifecycleHooksAndOptions(t *testing.T) {
	for _, parented := range []bool{true, false} {
		t.Run(fmt.Sprint(parented), func(t *testing.T) {
			p := testkit.New(t)
			cfg, err := pgxpool.ParseConfig("postgres://user:SECRET@localhost/appdb")
			if err != nil {
				t.Fatal(err)
			}
			err = InstrumentConnConfig(cfg.ConnConfig, p, WithQuerySpanNamePrefix(true), WithConnectionDetails(false), WithRawStatement(true), WithSpanNameFormatter(func(context.Context, string) string { return "lookup" }))
			if err != nil {
				t.Fatal(err)
			}
			h := cfg.ConnConfig.Tracer.(*tracer)
			ctx := context.Background()
			var parent trace.Span
			if parented {
				ctx, parent = p.TP.Tracer("app").Start(ctx, "parent")
				defer parent.End()
			}
			q := h.TraceQueryStart(ctx, nil, native.TraceQueryStartData{SQL: "SELECT 1"})
			h.TraceQueryEnd(q, nil, native.TraceQueryEndData{})
			prep := h.TracePrepareStart(ctx, nil, native.TracePrepareStartData{SQL: "SELECT 1", Name: "statement"})
			h.TracePrepareEnd(prep, nil, native.TracePrepareEndData{})
			conn := h.TraceConnectStart(ctx, native.TraceConnectStartData{ConnConfig: cfg.ConnConfig})
			h.TraceConnectEnd(conn, native.TraceConnectEndData{Err: &pgconn.PgError{Code: "08006", Message: "SECRET"}})
			batch := &native.Batch{}
			batch.Queue("SELECT 1")
			b := h.TraceBatchStart(ctx, nil, native.TraceBatchStartData{Batch: batch})
			h.TraceBatchQuery(b, nil, native.TraceBatchQueryData{SQL: "SELECT 1"})
			h.TraceBatchEnd(b, nil, native.TraceBatchEndData{})
			copy := h.TraceCopyFromStart(ctx, nil, native.TraceCopyFromStartData{TableName: native.Identifier{"users"}})
			h.TraceCopyFromEnd(copy, nil, native.TraceCopyFromEndData{})
			pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			acquire := h.TraceAcquireStart(ctx, pool, pgxpool.TraceAcquireStartData{})
			h.TraceAcquireEnd(acquire, pool, pgxpool.TraceAcquireEndData{Err: context.Canceled})
			if err = ObservePool(pool, p, WithPoolStatsInterval(0), nil, WithDBName("orders"), WithMetricAttributes(attribute.String("app.region", "test"))); err != nil {
				t.Fatal(err)
			}
			spans := p.Recorder.Ended()
			if len(spans) != 7 || spans[0].Name() != "query lookup" || spans[1].Name() != "prepare lookup" || spans[2].Status().Code != codes.Error {
				t.Fatalf("lifecycle spans: %v", spans)
			}
			for _, span := range spans {
				for _, a := range span.Attributes() {
					if a.Key == "server.address" {
						t.Fatal("connection option ignored")
					}
				}
			}
			if !strings.Contains(fmt.Sprint(p.Collect(t)), "pgxpool.") {
				t.Fatal("native pool metrics absent")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	p := testkit.New(t)
	h, _ := newTracer(p, []Option{WithAcquireTracing(false)})
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	a := h.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{})
	h.TraceAcquireEnd(a, nil, pgxpool.TraceAcquireEndData{})
	if len(p.Recorder.Ended()) != 0 {
		t.Fatal("disabled acquire ended parent")
	}
	parent.End()
	if InstrumentConfig(nil, nil) == nil || InstrumentConfig(&pgxpool.Config{}, nil) == nil || InstrumentConnConfig(nil, nil) == nil || ObservePool(nil, nil) == nil {
		t.Fatal("nil config accepted")
	}
	if _, err := newTracer(nil, []Option{WithStatementCapture(true)}); err == nil {
		t.Fatal("unsafe capture accepted")
	}
	if op("-- comment\nSELECT 1") != "SELECT" {
		t.Fatal("SQL parser")
	}
}

type failedMeterProvider struct {
	metric.MeterProvider
	err error
}

func (p failedMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return failedMeter{Meter: metricnoop.NewMeterProvider().Meter("failure"), err: p.err}
}

type failedMeter struct {
	metric.Meter
	err error
}

func (m failedMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, m.err
}
func (m failedMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, m.err
}
func TestMetricInitializationFailureIsReturned(t *testing.T) {
	failure := errors.New("failed instrument")
	p := testkit.New(t)
	provider := providerWithMeter{Provider: p, mp: failedMeterProvider{err: failure}}
	cfg, _ := native.ParseConfig("")
	if err := InstrumentConnConfig(cfg, provider); !errors.Is(err, failure) || cfg.Tracer != nil {
		t.Fatal("installed broken tracer", err)
	}
}

type providerWithMeter struct {
	*testkit.Provider
	mp metric.MeterProvider
}

func (p providerWithMeter) MeterProvider() metric.MeterProvider { return p.mp }

func TestQueryParameterPrivacyAndPreparedStatementMetadata(t *testing.T) {
	for _, parented := range []bool{false, true} {
		for _, mode := range []string{"disabled", "sanitized", "raw", "filtered", "bounded"} {
			t.Run(fmt.Sprint(parented, mode), func(t *testing.T) {
				p := testkit.New(t)
				var options []Option
				switch mode {
				case "sanitized":
					options = append(options, WithQueryParameters(func(values []string) []string {
						if values[0] != "SECRET" {
							t.Fatal("native parameter input")
						}
						return []string{"[redacted]"}
					}))
				case "raw":
					options = append(options, WithRawQueryParameters(true))
				case "filtered":
					options = append(options, WithRawQueryParameters(true), WithQueryParameterFilter(func(string) bool { return false }))
				case "bounded":
					options = append(options, WithQueryParameters(func([]string) []string {
						values := make([]string, 40)
						for i := range values {
							values[i] = strings.Repeat("x", 5000)
						}
						return values
					}))
				}
				tracer, err := newTracer(p, options)
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if parented {
					var parent trace.Span
					ctx, parent = p.TP.Tracer("app").Start(ctx, "parent")
					defer parent.End()
				}
				query := tracer.TraceQueryStart(ctx, nil, native.TraceQueryStartData{SQL: "SELECT $1", Args: []any{"SECRET"}})
				tracer.TraceQueryEnd(query, nil, native.TraceQueryEndData{})
				prepared := tracer.TracePrepareStart(ctx, nil, native.TracePrepareStartData{SQL: "SELECT $1", Name: "find_order"})
				tracer.TracePrepareEnd(prepared, nil, native.TracePrepareEndData{})
				spans := p.Recorder.Ended()
				var values []string
				for _, a := range spans[0].Attributes() {
					if a.Key == "pgx.query.parameters" {
						values = a.Value.AsStringSlice()
					}
				}
				switch mode {
				case "disabled", "filtered":
					if len(values) != 0 {
						t.Fatal("parameters should be omitted", values)
					}
				case "sanitized":
					if len(values) != 1 || values[0] != "[redacted]" {
						t.Fatal(values)
					}
				case "raw":
					if len(values) != 1 || values[0] != "SECRET" {
						t.Fatal(values)
					}
				case "bounded":
					size := 0
					for _, v := range values {
						size += len(v)
					}
					if len(values) != 32 || size != 4096 {
						t.Fatal("capture bounds", len(values), size)
					}
				}
				found := false
				for _, a := range spans[1].Attributes() {
					if a.Key == "pgx.prepare_stmt.name" && a.Value.AsString() == "find_order" {
						found = true
					}
				}
				if !found {
					t.Fatal("prepared statement metadata lost")
				}
				if mode != "raw" {
					p.AssertPrivate(t, "SECRET")
				}
			})
		}
	}
}
