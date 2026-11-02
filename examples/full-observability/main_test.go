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
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/muhammadluth/goslogx"
	goslogxredis "github.com/muhammadluth/goslogx/plugins/cache/redis"
	goslogxbun "github.com/muhammadluth/goslogx/plugins/database/bun"
	goslogxhttp "github.com/muhammadluth/goslogx/plugins/http/std"
	goslogxnats "github.com/muhammadluth/goslogx/plugins/messaging/nats"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Uses a real, in-process NATS server and independent producer/worker connections.
// No Docker, Collector or external service is required for this topology test.
func TestMessagingDistributedTopology(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	var apiLogs bytes.Buffer
	api := goslogx.New(goslogx.WithServiceName("payment-api"), goslogx.WithOutput(&apiLogs), goslogx.WithTracerProvider(tp), goslogx.WithMeterProvider(mp))
	workerLogger := api
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db := bun.NewDB(sqlDB, pgdialect.New())
	defer db.Close()
	if err = goslogxbun.Instrument(db, api); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	redisServer := miniredis.RunT(t)
	redisServer.Set("inventory", "SECRET")
	cache := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer cache.Close()
	stop := make(chan struct{})
	defer close(stop)
	if err = goslogxredis.Instrument(cache, api, goslogxredis.WithCloseChan(stop)); err != nil {
		t.Fatal(err)
	}
	if err = cache.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	recorder.Reset()
	ns, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	defer ns.Shutdown()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS readiness")
	}
	producerConn, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer producerConn.Close()
	workerConn, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer workerConn.Close()
	producer, err := goslogxnats.Wrap(producerConn, api, goslogxnats.WithDestinationNormalizer(func(string) string { return "payments" }))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	worker, err := goslogxnats.Wrap(workerConn, workerLogger, goslogxnats.WithErrorHandler(func(err error) { done <- err }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = worker.Subscribe(context.Background(), "payments.created", func(ctx context.Context, msg *nats.Msg) error {
		workerLogger.InfoContext(ctx, "", "worker", goslogx.MESSSAGE_TYPE_REQUEST, "payment received", nil)
		_, e := db.ExecContext(ctx, "UPDATE orders SET status = ? WHERE id = ?", "processed", "SECRET")
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = workerConn.Flush(); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(goslogxhttp.NewHandler(api, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), "inventory", goslogxhttp.WithRoute("/health")))
	defer upstream.Close()
	client := &http.Client{Transport: goslogxhttp.NewTransport(api, http.DefaultTransport)}
	defer client.CloseIdleConnections()
	app := newApp(api, db, cache, client, upstream.URL+"/health?token=SECRET", func(ctx context.Context) error { return producer.Publish(ctx, "payments.created", []byte("SECRET")) })
	req := httptest.NewRequest("POST", "http://localhost/payments", nil)
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatal(res.StatusCode)
	}
	// Wait for the processing span, which ends after the callback returns.
	deadline := time.Now().Add(5 * time.Second)
	for {
		found := false
		for _, s := range recorder.Ended() {
			if s.SpanKind() == trace.SpanKindConsumer && strings.HasSuffix(s.Name(), " process") {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case err := <-done:
			t.Fatal(err)
		default:
		}
		if time.Now().After(deadline) {
			names := []string{}
			for _, s := range recorder.Ended() {
				names = append(names, fmt.Sprint(s.Name(), " ", s.SpanKind()))
			}
			t.Fatalf("worker completion timeout: %v", names)
		}
		time.Sleep(time.Millisecond)
	}
	var pub, process, workerDB, inbound, appSpan, outbound, remote, cacheSpan sdktrace.ReadOnlySpan
	for _, s := range recorder.Ended() {
		if s.SpanContext().TraceID().String() != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("trace continuity: %s", s.Name())
		}
		if strings.Contains(fmt.Sprint(s.Name(), s.Attributes(), s.Events(), s.Status()), "SECRET") {
			t.Fatal("sensitive telemetry")
		}
		switch {
		case s.Name() == "POST /payments":
			inbound = s
		case s.Name() == "payment.process":
			appSpan = s
		case s.Name() == "inventory":
			remote = s
		case s.InstrumentationScope().Name == goslogxhttp.ScopeName && s.SpanKind() == trace.SpanKindClient:
			outbound = s
		case s.InstrumentationScope().Name == goslogxredis.ScopeName:
			cacheSpan = s
		case s.SpanKind() == trace.SpanKindProducer:
			pub = s
		case s.SpanKind() == trace.SpanKindConsumer && strings.HasSuffix(s.Name(), " receive"):
			t.Fatal("unexpected synthetic receive span")
		case s.SpanKind() == trace.SpanKindConsumer && strings.HasSuffix(s.Name(), " process"):
			process = s
		case s.Name() == "UPDATE":
			workerDB = s
		}
	}
	if pub == nil || process == nil || workerDB == nil {
		t.Fatalf("missing topology spans: %v", recorder.Ended())
	}
	if process.Parent().SpanID() != pub.SpanContext().SpanID() || !process.Parent().IsRemote() || workerDB.Parent().SpanID() != process.SpanContext().SpanID() {
		t.Fatal("messaging parent chain")
	}
	if inbound == nil || appSpan == nil || outbound == nil || remote == nil || cacheSpan == nil {
		t.Fatal("missing HTTP/cache spans")
	}
	if appSpan.Parent().SpanID() != inbound.SpanContext().SpanID() || pub.Parent().SpanID() != appSpan.SpanContext().SpanID() || cacheSpan.Parent().SpanID() != appSpan.SpanContext().SpanID() || outbound.Parent().SpanID() != appSpan.SpanContext().SpanID() || remote.Parent().SpanID() != outbound.SpanContext().SpanID() || !remote.Parent().IsRemote() {
		t.Fatal("HTTP/cache parent chain")
	}
	var entry map[string]any
	decoder := json.NewDecoder(bytes.NewReader(apiLogs.Bytes()))
	for decoder.More() {
		var candidate map[string]any
		if err := decoder.Decode(&candidate); err != nil {
			t.Fatal(err)
		}
		if candidate["module"] == "worker" {
			entry = candidate
		}
	}
	if entry == nil {
		t.Fatal("missing worker log")
	}
	if entry["obs_trace_id"] != process.SpanContext().TraceID().String() || entry["obs_span_id"] != process.SpanContext().SpanID().String() {
		t.Fatal("worker log correlation")
	}
	var data metricdata.ResourceMetrics
	if err = reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			names[m.Name] = true
		}
	}
	for _, name := range []string{"messaging.client.sent.messages", "messaging.client.consumed.messages", "messaging.process.duration", "go.sql.query_timing", "http.client.request.duration"} {
		if !names[name] {
			t.Errorf("missing %s", name)
		}
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("sensitive metric")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
