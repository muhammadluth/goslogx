package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type providers struct {
	tp trace.TracerProvider
	mp metric.MeterProvider
}

func (p *providers) TracerProvider() trace.TracerProvider { return p.tp }
func (p *providers) MeterProvider() metric.MeterProvider  { return p.mp }

func telemetry(t *testing.T) (*providers, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); _ = mp.Shutdown(context.Background()) })
	return &providers{tp, mp}, recorder, reader
}

func TestClients(t *testing.T) {
	for _, kind := range []string{"client", "cluster", "ring", "universal"} {
		t.Run(kind, func(t *testing.T) {
			server := miniredis.RunT(t)
			server.RequireAuth("credential-secret")
			var client goredis.UniversalClient
			switch kind {
			case "client":
				client = goredis.NewClient(&goredis.Options{Addr: server.Addr(), Password: "credential-secret"})
			case "cluster":
				client = goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{server.Addr()}, Password: "credential-secret", ClusterSlots: func(context.Context) ([]goredis.ClusterSlot, error) {
					return []goredis.ClusterSlot{{Start: 0, End: 16383, Nodes: []goredis.ClusterNode{{Addr: server.Addr()}}}}, nil
				}})
			case "ring":
				client = goredis.NewRing(&goredis.RingOptions{Addrs: map[string]string{"shard-secret": server.Addr()}, Password: "credential-secret"})
			case "universal":
				client = goredis.NewUniversalClient(&goredis.UniversalOptions{Addrs: []string{server.Addr()}, Password: "credential-secret"})
			}
			defer client.Close()
			p, recorder, reader := telemetry(t)
			closeChan := make(chan struct{})
			defer close(closeChan)
			if err := Instrument(client, p, WithCloseChan(closeChan)); err != nil {
				t.Fatal(err)
			}
			ctx, parent := p.tp.Tracer("test").Start(context.Background(), "parent")
			if err := client.Set(ctx, "key-secret", "value-secret", 0).Err(); err != nil {
				t.Fatal(err)
			}
			recorder.Reset()
			if got, err := client.Get(ctx, "key-secret").Result(); err != nil || got != "value-secret" {
				t.Fatalf("get: %q %v", got, err)
			}
			spans := recorder.Ended()
			// All supported clients must preserve parent context.
			{
				if len(spans) != 1 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || spans[0].SpanContext().TraceID() != parent.SpanContext().TraceID() {
					t.Fatalf("parent propagation: %v", spans)
				}
			}
			if err := client.Get(ctx, "missing-secret").Err(); !errors.Is(err, goredis.Nil) {
				t.Fatal(err)
			}
			spans = recorder.Ended()
			if spans[len(spans)-1].Status().Code == codes.Error {
				t.Fatal("redis.Nil marked as error")
			}
			if err := client.Do(ctx, "command-secret", "arg-secret").Err(); err == nil {
				t.Fatal("expected server error")
			}
			spans = recorder.Ended()
			if spans[len(spans)-1].Status().Code != codes.Error {
				t.Fatal("error not marked")
			}
			pipelineSupported := true
			if pipelineSupported {
				_, err := client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
					pipe.Get(ctx, "key-secret")
					pipe.Do(ctx, "pipeline-secret", "arg-secret")
					return nil
				})
				if err == nil {
					t.Fatal("expected pipeline error")
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						if err := client.Get(ctx, "key-secret").Err(); err != nil {
							t.Error(err)
						}
					}
				}()
			}
			wg.Wait()
			parent.End()
			pipeline := false
			for _, span := range recorder.Ended() {
				if span.Name() == "parent" {
					continue
				}
				if span.InstrumentationScope().Name != ScopeName {
					t.Fatal("wrong scope")
				}
				if span.SpanKind() != trace.SpanKindClient {
					t.Fatal("wrong kind")
				}
				if span.Name() == "redis.pipeline" {
					pipeline = true
				}
				text := fmt.Sprintf("%s %v %v %v %v", span.Name(), span.Attributes(), span.Events(), span.Status(), span.Links())
				if strings.Contains(text, "secret") || strings.Contains(text, server.Addr()) {
					t.Fatalf("span leaked: %s", text)
				}
				if len(span.Events()) != 0 || span.Status().Description != "" {
					t.Fatal("raw error exported")
				}
			}
			if pipelineSupported && !pipeline {
				t.Fatal("missing pipeline span")
			}
			// Initial Ring shards must expose metrics too.
			{
				var data metricdata.ResourceMetrics
				if err := reader.Collect(ctx, &data); err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), server.Addr()) {
					t.Fatalf("metrics leaked: %s", raw)
				}
				duration, failures, pool := false, false, false
				for _, scope := range data.ScopeMetrics {
					if scope.Scope.Name != ScopeName {
						t.Fatal("wrong metric scope")
					}
					for _, m := range scope.Metrics {
						if m.Name == "db.client.connections.usage" {
							pool = true
						}
						if m.Name == "db.client.connections.use_time" {
							h, ok := m.Data.(metricdata.Histogram[float64])
							if !ok {
								t.Fatalf("wrong duration type: %T", m.Data)
							}
							for _, dp := range h.DataPoints {
								if dp.Count > 0 {
									duration = true
								}
								status, _ := dp.Attributes.Value("status")
								if status.AsString() == "error" && dp.Count > 0 {
									failures = true
								}
							}
						}
					}
				}
				if !duration || !failures || !pool {
					t.Fatalf("metrics: duration=%v failures=%v pool=%v", duration, failures, pool)
				}
			}
		})
	}
}

func TestNoGlobalFallback(t *testing.T) {
	global, recorder, reader := telemetry(t)
	oldTP, oldMP := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(global.tp)
	otel.SetMeterProvider(global.mp)
	defer otel.SetTracerProvider(oldTP)
	defer otel.SetMeterProvider(oldMP)
	server := miniredis.RunT(t)
	var typedNil *providers
	for _, p := range []*providers{nil, typedNil, {}, {tp: global.tp}, {mp: global.mp}} {
		client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
		local, localSpans, localReader := telemetry(t)
		if p != nil && p.tp != nil {
			p = &providers{tp: local.tp}
		}
		if p != nil && p.mp != nil {
			p = &providers{mp: local.mp}
		}
		if err := Instrument(client, p); err != nil {
			t.Fatal(err)
		}
		if err := client.Ping(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		if p != nil && p.tp != nil && len(localSpans.Ended()) == 0 {
			t.Fatal("missing tracing-only output")
		}
		var data metricdata.ResourceMetrics
		if err := localReader.Collect(context.Background(), &data); err != nil {
			t.Fatal(err)
		}
		if p != nil && p.mp != nil && len(data.ScopeMetrics) == 0 {
			t.Fatal("missing metrics-only output")
		}
		client.Close()
	}
	if len(recorder.Ended()) != 0 {
		t.Fatal("used global tracing")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.ScopeMetrics) != 0 {
		t.Fatal("used global metrics")
	}
}

func TestCapabilityParityOptions(t *testing.T) {
	t.Run("enriched tracing", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
		defer client.Close()
		p, recorder, _ := telemetry(t)
		if err := Instrument(client, p,
			WithMetrics(false),
			WithDBStatement(true),
			WithCallerEnabled(true),
			WithAttributes(attribute.String("deployment.environment.name", "test")),
		); err != nil {
			t.Fatal(err)
		}
		if err := client.Set(context.Background(), "visible-key", "visible-value", 0).Err(); err != nil {
			t.Fatal(err)
		}
		var command sdktrace.ReadOnlySpan
		for _, span := range recorder.Ended() {
			if span.Name() == "redis.command" {
				command = span
			}
		}
		if command == nil {
			t.Fatal("missing command span")
		}
		attrs := spanAttributes(command)
		if got := attrs["db.statement"]; !strings.Contains(got, "visible-key") || !strings.Contains(got, "visible-value") {
			t.Fatalf("statement option not preserved: %q", got)
		}
		if attrs["code.function"] == "" || attrs["code.filepath"] == "" || attrs["code.lineno"] == "" {
			t.Fatalf("caller option not preserved: %v", attrs)
		}
		if attrs["deployment.environment.name"] != "test" {
			t.Fatalf("custom attribute not preserved: %v", attrs)
		}
	})

	t.Run("filters", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
		defer client.Close()
		p, recorder, _ := telemetry(t)
		if err := Instrument(client, p,
			WithMetrics(false),
			WithDialFilter(true),
			WithCommandFilter(func(cmd goredis.Cmder) bool { return cmd.Name() == "get" }),
			WithCommandsFilter(func([]goredis.Cmder) bool { return true }),
		); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if err := client.Set(ctx, "key", "value", 0).Err(); err != nil {
			t.Fatal(err)
		}
		for _, span := range recorder.Ended() {
			if span.Name() == "redis.dial" {
				t.Fatal("dial filter did not suppress dial span")
			}
		}
		recorder.Reset()
		if err := client.Get(ctx, "key").Err(); err != nil {
			t.Fatal(err)
		}
		if spans := recorder.Ended(); len(spans) != 0 {
			t.Fatalf("command filter produced %d spans", len(spans))
		}
		recorder.Reset()
		if _, err := client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.Get(ctx, "key")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for _, span := range recorder.Ended() {
			if span.Name() == "redis.pipeline" {
				t.Fatal("pipeline filter did not suppress pipeline span")
			}
		}
	})

	t.Run("signal modes", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			tracing    bool
			metrics    bool
			wantSpans  bool
			wantMetric bool
		}{
			{name: "tracing only", tracing: true, metrics: false, wantSpans: true},
			{name: "metrics only", tracing: false, metrics: true, wantMetric: true},
			{name: "disabled", tracing: false, metrics: false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				server := miniredis.RunT(t)
				client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
				defer client.Close()
				p, recorder, reader := telemetry(t)
				closeChan := make(chan struct{})
				defer close(closeChan)
				if err := Instrument(client, p, WithTracing(tc.tracing), WithMetrics(tc.metrics), WithCloseChan(closeChan), WithPoolName("primary")); err != nil {
					t.Fatal(err)
				}
				if err := client.Ping(context.Background()).Err(); err != nil {
					t.Fatal(err)
				}
				if got := len(recorder.Ended()) > 0; got != tc.wantSpans {
					t.Fatalf("spans=%v want %v", got, tc.wantSpans)
				}
				var data metricdata.ResourceMetrics
				if err := reader.Collect(context.Background(), &data); err != nil {
					t.Fatal(err)
				}
				if got := len(data.ScopeMetrics) > 0; got != tc.wantMetric {
					t.Fatalf("metrics=%v want %v", got, tc.wantMetric)
				}
			})
		}
	})
}

func TestAttributePrivacyValidation(t *testing.T) {
	attrs := safeConfiguredAttributes([]attribute.KeyValue{
		attribute.String("service.namespace", "payments"),
		attribute.String("auth.token", "secret"),
		attribute.String("db.statement", "SET key value"),
	})
	if len(attrs) != 1 || attrs[0].Key != "service.namespace" {
		t.Fatalf("unexpected safe attributes: %v", attrs)
	}
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	result := make(map[string]string, len(span.Attributes()))
	for _, attr := range span.Attributes() {
		result[string(attr.Key)] = attr.Value.Emit()
	}
	return result
}

type unsupported struct{ goredis.UniversalClient }

func TestInvalidClients(t *testing.T) {
	var client *goredis.Client
	var cluster *goredis.ClusterClient
	var ring *goredis.Ring
	for _, c := range []goredis.UniversalClient{nil, client, cluster, ring, unsupported{}} {
		if err := Instrument(c, nil); err == nil {
			t.Fatal("expected validation error")
		}
	}
}

func (p *providers) TextMapPropagator() propagation.TextMapPropagator {
	return propagation.TraceContext{}
}

func BenchmarkInstrumentation(b *testing.B) {
	server := miniredis.NewMiniRedis()
	if err := server.Start(); err != nil {
		b.Fatal(err)
	}
	defer server.Close()
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer client.Close()
	if err := Instrument(client, nil); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for b.Loop() {
		if err := client.Ping(ctx).Err(); err != nil {
			b.Fatal(err)
		}
	}
}
