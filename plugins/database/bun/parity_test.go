package bun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/extra/bunotel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNativeMetricsAndIndependentSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			driverName, _ := testkit.RegisterSQL(t)
			raw, err := sql.Open(driverName, "")
			if err != nil {
				t.Fatal(err)
			}
			db := native.NewDB(raw, pgdialect.New())
			defer db.Close()
			err = Instrument(db, p, WithTracing(signals[0]), WithMetrics(signals[1]), WithDBName("orders"), WithPoolName("primary"), WithAttributes(attribute.String("db.name", "orders"), attribute.String("app.token", "SECRET")), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }), WithCallerEnabled(true), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")), WithSpanNameFormatter(func(*native.QueryEvent) string { return "lookup" }))
			if err != nil {
				t.Fatal(err)
			}
			if Instrument(db, p) == nil {
				t.Fatal("duplicate hook accepted")
			}
			if _, err = db.ExecContext(context.Background(), "SELECT ?", "SECRET"); err != nil {
				t.Fatal(err)
			}
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] {
				t.Fatal("tracing toggle")
			}
			if signals[0] {
				if spans[0].Name() != "lookup" {
					t.Fatal("native formatter")
				}
				attrs := fmt.Sprint(spans[0].Attributes())
				for _, want := range []string{"SELECT ?", "db.name", "code.function", "app.component"} {
					if !strings.Contains(attrs, want) {
						t.Fatal("attribute lost", want, attrs)
					}
				}
			}
			data := p.Collect(t)
			if (len(data.ScopeMetrics) > 0) != signals[1] {
				t.Fatal("metrics toggle")
			}
			queries := uint64(0)
			for _, scope := range data.ScopeMetrics {
				for _, m := range scope.Metrics {
					if m.Name == "db.client.operation.duration" {
						t.Fatal("legacy metric enabled by default")
					}
					if m.Name == "go.sql.query_timing" {
						if m.Unit != "milliseconds" {
							t.Fatal("native unit changed")
						}
						for _, dp := range m.Data.(metricdata.Histogram[int64]).DataPoints {
							queries += dp.Count
						}
					}
				}
			}
			if signals[1] && queries != 1 {
				t.Fatal("duplicate/missing native timing", queries)
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
}

func TestQueryParityWithBunotel(t *testing.T) {
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	for _, direct := range []bool{true, false} {
		name, _ := testkit.RegisterSQL(t)
		raw, err := sql.Open(name, "")
		if err != nil {
			t.Fatal(err)
		}
		db := native.NewDB(raw, pgdialect.New())
		if direct {
			db.AddQueryHook(queryHook{bunotel.NewQueryHook(bunotel.WithTracerProvider(p.TP), bunotel.WithMeterProvider(p.MP))})
		} else if err = Instrument(db, p, WithRawStatement(true)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
	}
	a, b := p.Recorder.Ended()[0], p.Recorder.Ended()[1]
	if a.Name() != b.Name() || a.SpanKind() != b.SpanKind() || !a.Parent().Equal(b.Parent()) || a.Status() != b.Status() {
		t.Fatal("native span semantics changed")
	}
}

type failedProvider struct {
	*testkit.Provider
	mp metric.MeterProvider
}

func (p failedProvider) MeterProvider() metric.MeterProvider { return p.mp }

type failedMeterProvider struct {
	metric.MeterProvider
	err error
}

func (p failedMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return failedMeter{Meter: metricnoop.NewMeterProvider().Meter("failed"), err: p.err}
}

type failedMeter struct {
	metric.Meter
	err error
}

func (m failedMeter) Int64Histogram(string, ...metric.Int64HistogramOption) (metric.Int64Histogram, error) {
	return nil, m.err
}
func TestQueryInstrumentFailureDoesNotInstallBrokenHook(t *testing.T) {
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	raw, _ := sql.Open(name, "")
	db := native.NewDB(raw, pgdialect.New())
	defer db.Close()
	failure := errors.New("failed histogram")
	if err := Instrument(db, failedProvider{p, failedMeterProvider{err: failure}}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := Instrument(db, p); err != nil {
		t.Fatal("failed construction must be retryable", err)
	}
	if _, err := db.ExecContext(context.Background(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if len(p.Recorder.Ended()) != 1 {
		t.Fatal("failed constructor retained hook")
	}
}
