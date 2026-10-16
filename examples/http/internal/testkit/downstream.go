package testkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type Builder func(goslogx.TelemetryProvider, http.Handler) http.Handler

func RunDownstream(t *testing.T, name string, build Builder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	var logs bytes.Buffer
	logger := goslogx.New(goslogx.WithOutput(&logs), goslogx.WithTracerProvider(tp), goslogx.WithMeterProvider(mp))
	ns, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	defer ns.Shutdown()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS readiness")
	}
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	workerConn, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer workerConn.Close()
	sub, err := workerConn.SubscribeSync("orders")
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	if err = workerConn.Flush(); err != nil {
		t.Fatal(err)
	}
	messaging, err := goslogxnats.Wrap(nc, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Run(name, func(t *testing.T) {
		logs.Reset()
		sqlDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db := bun.NewDB(sqlDB, pgdialect.New())
		defer db.Close()
		if err = goslogxbun.Instrument(db, logger); err != nil {
			t.Fatal(err)
		}
		mock.ExpectExec("INSERT").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		rs := miniredis.RunT(t)
		cache := redis.NewClient(&redis.Options{Addr: rs.Addr()})
		defer cache.Close()
		stop := make(chan struct{})
		defer close(stop)
		if err = goslogxredis.Instrument(cache, logger, goslogxredis.WithCloseChan(stop)); err != nil {
			t.Fatal(err)
		}
		if err = cache.Ping(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		recorder.Reset()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
		defer upstream.Close()
		client := goslogxhttp.InstrumentClient(&http.Client{Timeout: time.Second}, logger)
		defer client.CloseIdleConnections()
		h := build(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger.InfoContext(ctx, "", "orders", goslogx.MESSSAGE_TYPE_REQUEST, "received", nil)
			if _, err := db.ExecContext(ctx, "INSERT INTO orders VALUES (?)", "SECRET"); err != nil {
				t.Error(err)
			}
			if err := cache.Set(ctx, "SECRET", "SECRET", 0).Err(); err != nil {
				t.Error(err)
			}
			if err := messaging.Publish(ctx, "orders", []byte("SECRET")); err != nil {
				t.Error(err)
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", upstream.URL+"/?token=SECRET", nil)
			res, err := client.Do(req)
			if err != nil {
				t.Error(err)
			} else {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}
			w.WriteHeader(201)
		}))
		req := httptest.NewRequest("POST", "http://localhost/orders/SECRET?token=SECRET", nil)
		req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatal(w.Code)
		}
		msg, err := sub.NextMsg(5 * time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err = messaging.Process(context.Background(), msg, func(ctx context.Context, _ *nats.Msg) error {
			_, err := db.ExecContext(ctx, "UPDATE orders SET status = ?", "SECRET")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		var serverSC, producerSC, receiveSC, processSC trace.SpanContext
		for _, s := range recorder.Ended() {
			if s.SpanContext().TraceID().String() != "0123456789abcdef0123456789abcdef" {
				t.Fatal("trace continuity", s.Name())
			}
			if strings.Contains(fmt.Sprint(s.Attributes(), s.Events(), s.Name(), s.Status()), "SECRET") {
				t.Fatal("sensitive trace")
			}
			if s.SpanKind() == trace.SpanKindServer {
				if serverSC.IsValid() {
					t.Fatal("duplicate server span")
				}
				serverSC = s.SpanContext()
			}
			if s.SpanKind() == trace.SpanKindProducer {
				producerSC = s.SpanContext()
			}
			if s.SpanKind() == trace.SpanKindConsumer {
				if strings.HasSuffix(s.Name(), " receive") {
					receiveSC = s.SpanContext()
				} else if strings.HasSuffix(s.Name(), " process") {
					processSC = s.SpanContext()
				}
			}
		}
		counts := map[string]int{}
		for _, s := range recorder.Ended() {
			if s.SpanKind() == trace.SpanKindClient || s.SpanKind() == trace.SpanKindProducer {
				expected := serverSC.SpanID()
				if s.Name() == "UPDATE" {
					expected = processSC.SpanID()
				}
				if s.Parent().SpanID() != expected {
					t.Fatal("downstream parent", s.Name())
				}
				counts[s.InstrumentationScope().Name]++
			}
			if s.SpanContext().SpanID() == processSC.SpanID() && s.Parent().SpanID() != receiveSC.SpanID() {
				t.Fatal("process parent")
			}
			if s.SpanContext().SpanID() == receiveSC.SpanID() && (!s.Parent().IsRemote() || s.Parent().SpanID() != producerSC.SpanID()) {
				t.Fatal("remote propagation")
			}
		}
		for _, scope := range []string{"github.com/muhammadluth/goslogx/plugins/database/bun", goslogxredis.ScopeName, goslogxnats.ScopeName, goslogxhttp.ScopeName} {
			if counts[scope] == 0 {
				t.Error("missing downstream", scope)
			}
		}
		var entry map[string]any
		if err = json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["obs_span_id"] != serverSC.SpanID().String() {
			t.Fatal("log correlation")
		}
		var data metricdata.ResourceMetrics
		if err = reader.Collect(context.Background(), &data); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(fmt.Sprint(data), "SECRET") {
			t.Fatal("sensitive metric")
		}
		if err = mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}
