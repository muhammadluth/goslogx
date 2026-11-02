package bun

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/muhammadluth/goslogx"
	upbun "github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestQueriesProvidersPrivacy(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db := upbun.NewDB(sqlDB, pgdialect.New())
	defer db.Close()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	// Preserve the core singleton New behavior: configure this test process once.
	logger := goslogx.New(goslogx.WithTracerProvider(tp), goslogx.WithMeterProvider(mp))
	if err := Instrument(db, logger, WithDBName("payment"), WithQueryStatement(false), WithCompatibilityMetrics(true)); err != nil {
		t.Fatal(err)
	}
	ctx, parent := logger.StartSpan(context.Background(), "parent")
	var value int
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1))
	if err := db.NewSelect().ColumnExpr("1").Scan(ctx, &value); err != nil || value != 1 {
		t.Fatalf("select: %d %v", value, err)
	}
	type payment struct{ ID int }
	mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(1, 1))
	if _, err := db.NewInsert().Model(&payment{ID: 1}).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("SECRET").WillReturnError(errors.New("SECRET password=SECRET"))
	if _, err := db.ExecContext(ctx, "SECRET ?", "SECRET"); err == nil {
		t.Fatal("expected error")
	}
	parent.End()
	spans := recorder.Ended()
	if len(spans) != 4 {
		t.Fatalf("spans: %d", len(spans))
	}
	for _, span := range spans[:3] {
		if span.Parent().SpanID() != parent.SpanContext().SpanID() || span.InstrumentationScope().Name != instrumentationScope {
			t.Fatal("parent/scope")
		}
		if strings.Contains(fmt.Sprint(span.Name(), span.Attributes(), span.Events(), span.Status()), "SECRET") {
			t.Fatal("secret leak")
		}
	}
	if spans[2].Name() != "QUERY" || spans[2].Status().Code != codes.Error || len(spans[2].Events()) != 1 {
		t.Fatal("error policy")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("metric secret")
	}
	count, errorsCount, pool := uint64(0), uint64(0), false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "db.client.operation.duration":
				for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					count += point.Count
					if _, ok := point.Attributes.Value("error.type"); ok {
						errorsCount += point.Count
					}
					for _, a := range point.Attributes.ToSlice() {
						if a.Key != "db.operation.name" && a.Key != "db.system.name" && a.Key != "db.namespace" && a.Key != "error.type" {
							t.Fatal(a)
						}
					}
				}
			case "db.client.connection.count":
				pool = true
			}
		}
	}
	if count != 3 || errorsCount != 1 || !pool {
		t.Fatalf("count=%d errors=%d pool=%v", count, errorsCount, pool)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDisabledAndValidation(t *testing.T) {
	if Instrument(nil, nil) == nil {
		t.Fatal("nil DB")
	}
	for _, provider := range []goslogx.TelemetryProvider{nil, (*goslogx.Logger)(nil), goslogx.New()} {
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db := upbun.NewDB(sqlDB, pgdialect.New())
		if err := Instrument(db, provider); err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec("SELECT").WillReturnResult(sqlmock.NewResult(0, 0))
		if _, err := db.ExecContext(context.Background(), "SELECT 1"); err != nil {
			t.Fatal(err)
		}
		if Instrument(db, provider, WithQueryStatement(true)) == nil {
			t.Fatal("unsupported capture silently accepted")
		}
		db.Close()
	}
}

func BenchmarkHook(b *testing.B) {
	logger := goslogx.New()
	duration, _ := logger.Meter(instrumentationScope).Float64Histogram("db.client.operation.duration")
	h := &metricsHook{duration: duration, attributes: []attribute.KeyValue{attribute.String("db.namespace", "payment")}}
	event := &upbun.QueryEvent{Query: "SELECT 1"}
	for b.Loop() {
		ctx := h.BeforeQuery(context.Background(), event)
		h.AfterQuery(ctx, event)
	}
}
