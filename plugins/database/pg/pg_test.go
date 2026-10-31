package pg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type testQuery struct{ operation orm.QueryOp }

func (q testQuery) Operation() orm.QueryOp { return q.operation }

type testResult struct {
	affected int
	returned int
}

func (testResult) Model() orm.Model    { return nil }
func (r testResult) RowsAffected() int { return r.affected }
func (r testResult) RowsReturned() int { return r.returned }

func TestActualQueryHookWithoutServer(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	logger := goslogx.New(goslogx.WithTracerProvider(tp), goslogx.WithMeterProvider(mp))
	db := pg.Connect(&pg.Options{Addr: "SECRET", User: "SECRET", Password: "SECRET", MaxRetries: 0,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("SECRET credentials=SECRET")
		},
	})
	defer db.Close()
	if err := Instrument(db, logger, WithDBName("payment")); err != nil {
		t.Fatal(err)
	}
	ctx, parent := logger.StartSpan(context.Background(), "parent")
	if _, err := db.ExecContext(ctx, "SELECT ?", "SECRET"); err == nil {
		t.Fatal("expected injected dial error")
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans: %d", len(spans))
	}
	span := spans[0]
	if span.Parent().SpanID() != parent.SpanContext().SpanID() || span.Status().Code != codes.Error || span.InstrumentationScope().Name != instrumentationScope {
		t.Fatal("parent/status/scope")
	}
	if strings.Contains(fmt.Sprint(span.Name(), span.Attributes(), span.Events(), span.Status()), "SECRET") {
		t.Fatal("secret leaked")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("metric secret")
	}
	duration, pool := false, false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "db.client.operation.duration" {
				duration = true
				h := m.Data.(metricdata.Histogram[float64])
				if len(h.DataPoints) != 1 || h.DataPoints[0].Count != 1 {
					t.Fatal(h)
				}
			}
			if m.Name == "db.client.connection.count" {
				pool = true
			}
		}
	}
	if !duration || !pool {
		t.Fatal("missing metrics")
	}
}

func TestNoopAndCaptureValidation(t *testing.T) {
	if Instrument(nil, nil) == nil {
		t.Fatal("nil database accepted")
	}
	for _, p := range []goslogx.TelemetryProvider{nil, (*goslogx.Logger)(nil), goslogx.New()} {
		db := pg.Connect(&pg.Options{})
		if err := Instrument(db, p); err != nil {
			t.Fatal(err)
		}
		if Instrument(db, p, WithQueryStatement(true)) == nil {
			t.Fatal("unsupported capture silently accepted")
		}
		db.Close()
	}
}

func TestNotFoundHook(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	h := queryHook{tracer: tp.Tracer(instrumentationScope)}
	event := &pg.QueryEvent{Err: pg.ErrNoRows, StartTime: time.Now()}
	ctx, err := h.BeforeQuery(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.AfterQuery(ctx, event); err != nil {
		t.Fatal(err)
	}
	if recorder.Ended()[0].Status().Code == codes.Error {
		t.Fatal("not-found recorded as failure")
	}
}

func TestRowsAffectedUsesSafeUpstreamBehavior(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	h := queryHook{tracer: tp.Tracer(instrumentationScope)}
	event := &pg.QueryEvent{
		Query:     testQuery{operation: orm.SelectOp},
		Result:    testResult{affected: -1, returned: 2},
		StartTime: time.Now(),
	}
	ctx, err := h.BeforeQuery(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.AfterQuery(ctx, event); err != nil {
		t.Fatal(err)
	}
	attrs := recorder.Ended()[0].Attributes()
	set := attribute.NewSet(attrs...)
	rows, ok := set.Value("db.rows_affected")
	if !ok || rows.AsInt64() != 2 {
		t.Fatalf("rows attribute: %v", attrs)
	}
}
