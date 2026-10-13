package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/muhammadluth/goslogx"
	goslogxbun "github.com/muhammadluth/goslogx/plugins/bun"
	goslogxhttp "github.com/muhammadluth/goslogx/plugins/http"
	goslogxredis "github.com/muhammadluth/goslogx/plugins/redis"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestCorrelatedTraceAcrossPlugins(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	var output bytes.Buffer
	logger := goslogx.New(goslogx.WithServiceName("payment-api"), goslogx.WithOutput(&output), goslogx.WithTracerProvider(tp), goslogx.WithMeterProvider(mp), goslogx.WithInternalMetrics(true))
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	defer db.Close()
	if err := goslogxbun.Instrument(db, logger, goslogxbun.WithDBName("payment")); err != nil {
		t.Fatal(err)
	}
	server := miniredis.RunT(t)
	server.Set("inventory", "SECRET")
	cache := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer cache.Close()
	closeMetrics := make(chan struct{})
	defer close(closeMetrics)
	if err := goslogxredis.Instrument(cache, logger, goslogxredis.WithCloseChan(closeMetrics)); err != nil {
		t.Fatal(err)
	}
	// Warm up the client before measuring request parent/child relationships.
	if err := cache.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	recorder.Reset()
	upstream := httptest.NewServer(goslogxhttp.NewHandler(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "inventory", goslogxhttp.WithRoute("/health")))
	defer upstream.Close()
	client := &http.Client{Transport: goslogxhttp.NewTransport(logger, http.DefaultTransport)}
	defer client.CloseIdleConnections()
	app := newApp(logger, db, cache, client, upstream.URL+"/health?token=SECRET")
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	req := httptest.NewRequest("POST", "http://localhost/payments", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal(response.StatusCode)
	}
	var inbound, process, database, redisSpan, outbound, remote sdktrace.ReadOnlySpan
	for _, s := range recorder.Ended() {
		if s.SpanContext().TraceID().String() != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("different trace: %s", s.Name())
		}
		if strings.Contains(fmt.Sprint(s.Name(), s.Attributes(), s.Events(), s.Status()), "SECRET") {
			t.Fatal("secret in trace")
		}
		switch {
		case s.Name() == "POST /payments":
			inbound = s
		case s.Name() == "payment.process":
			process = s
		case s.InstrumentationScope().Name == "github.com/muhammadluth/goslogx/plugins/bun":
			database = s
		case s.InstrumentationScope().Name == goslogxredis.ScopeName:
			redisSpan = s
		case s.SpanKind() == trace.SpanKindClient:
			outbound = s
		case s.Name() == "inventory":
			remote = s
		}
	}
	if inbound == nil || process == nil || database == nil || redisSpan == nil || outbound == nil || remote == nil {
		t.Fatalf("missing spans: %v", recorder.Ended())
	}
	if !inbound.Parent().IsRemote() || process.Parent().SpanID() != inbound.SpanContext().SpanID() {
		t.Fatal("inbound parent chain")
	}
	for _, child := range []sdktrace.ReadOnlySpan{database, redisSpan, outbound} {
		if child.Parent().SpanID() != process.SpanContext().SpanID() {
			t.Fatalf("wrong parent: %s", child.Name())
		}
	}
	if remote.Parent().SpanID() != outbound.SpanContext().SpanID() || !remote.Parent().IsRemote() {
		t.Fatal("outbound propagation")
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["obs_trace_id"] != process.SpanContext().TraceID().String() || entry["obs_span_id"] != process.SpanContext().SpanID().String() {
		t.Fatalf("log correlation: %v", entry)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("secret in metrics")
	}
	names := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			names[m.Name] = true
		}
	}
	for _, name := range []string{"http.server.request.duration", "http.client.request.duration", "db.client.operation.duration", "db.client.connections.use_time", "goslogx.log.records"} {
		if !names[name] {
			t.Errorf("missing metric %s", name)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
