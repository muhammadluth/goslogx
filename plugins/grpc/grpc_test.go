package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	native "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type healthService struct {
	grpc_health_v1.UnimplementedHealthServer
	logger *goslogx.Logger
	t      *testing.T
}

func (s *healthService) Check(ctx context.Context, req *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "", "health", goslogx.MESSSAGE_TYPE_REQUEST, "health request", nil)
		if baggage.FromContext(ctx).Member("region").Value() != "test" {
			s.t.Error("custom propagation lost")
		}
	}
	if req.Service == "SECRET" {
		return nil, status.Error(codes.Internal, "SECRET")
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}
func connect(t *testing.T, p goslogx.TelemetryProvider, service *healthService, opts ...Option) *native.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := native.NewServer(native.StatsHandler(NewServerHandler(p, opts...)))
	grpc_health_v1.RegisterHealthServer(server, service)
	go server.Serve(listener)
	t.Cleanup(func() { server.Stop(); listener.Close() })
	conn, err := native.NewClient("passthrough:///local", native.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), native.WithTransportCredentials(insecure.NewCredentials()), native.WithStatsHandler(NewClientHandler(p, opts...)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
func TestRPCPropagationMetricsErrorsAndLogs(t *testing.T) {
	p := testkit.New(t)
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	var logs bytes.Buffer
	logger := goslogx.New(goslogx.WithOutput(&logs), goslogx.WithTracerProvider(p.TP), goslogx.WithMeterProvider(p.MP))
	client := grpc_health_v1.NewHealthClient(connect(t, logger, &healthService{logger: logger, t: t}, WithPropagator(prop), WithMethodNames(grpc_health_v1.Health_Check_FullMethodName), WithMetricAttributesFn(func(context.Context) []attribute.KeyValue {
		return []attribute.KeyValue{attribute.String("app.channel", "mobile"), attribute.String("app.user_id", "SECRET")}
	})))
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "request")
	member, _ := baggage.NewMember("region", "test")
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "SECRET")
	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "SECRET"}); status.Code(err) != codes.Internal {
		t.Fatal("native status changed", err)
	}
	parent.End()
	deadline := time.Now().Add(3 * time.Second)
	for len(p.Recorder.Ended()) < 5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	spans := p.Recorder.Ended()
	if len(spans) != 5 {
		t.Fatal("missing spans", len(spans))
	}
	clients := map[trace.SpanID]bool{}
	servers := map[string]bool{}
	failures := 0
	for _, s := range spans {
		if s.SpanContext().TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace continuity")
		}
		if s.SpanKind() == trace.SpanKindClient {
			clients[s.SpanContext().SpanID()] = true
			if s.Parent().SpanID() != parent.SpanContext().SpanID() {
				t.Fatal("client parent")
			}
		}
		if s.Status().Code == otelcodes.Error {
			failures++
		}
	}
	for _, s := range spans {
		if s.SpanKind() == trace.SpanKindServer {
			if !clients[s.Parent().SpanID()] || !s.Parent().IsRemote() {
				t.Fatal("remote server parent")
			}
			servers[s.SpanContext().SpanID().String()] = true
			if s.InstrumentationScope().Name != ScopeName {
				t.Fatal("scope")
			}
		}
	}
	if failures != 2 {
		t.Fatal("error semantics", failures)
	}
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	entries := 0
	for decoder.More() {
		var entry map[string]any
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry["obs_trace_id"] != parent.SpanContext().TraceID().String() || !servers[fmt.Sprint(entry["obs_span_id"])] {
			t.Fatal("log correlation")
		}
		entries++
	}
	if entries != 2 {
		t.Fatal("missing application logs")
	}
	data := fmt.Sprint(p.Collect(t))
	if !strings.Contains(data, "rpc.") || !strings.Contains(data, "mobile") {
		t.Fatal("RPC metrics")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestDisabledAndFilteredHandlers(t *testing.T) {
	for _, tc := range []struct {
		name            string
		opts            []Option
		traces, metrics bool
	}{
		{"disabled", []Option{WithTracing(false), WithMetrics(false)}, false, false},
		{"filter", []Option{WithFilter(func(*stats.RPCTagInfo) bool { return false })}, false, false},
		{"traces-only", []Option{WithMetrics(false)}, true, false},
		{"metrics-only", []Option{WithTracing(false)}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testkit.New(t)
			conn := connect(t, p, &healthService{}, tc.opts...)
			if _, err := grpc_health_v1.NewHealthClient(conn).Check(context.Background(), &grpc_health_v1.HealthCheckRequest{}); err != nil {
				t.Fatal(err)
			}
			if tc.traces {
				deadline := time.Now().Add(time.Second)
				for len(p.Recorder.Ended()) < 2 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
			}
			if (len(p.Recorder.Ended()) > 0) != tc.traces {
				t.Fatal("tracing toggle")
			}
			if data := p.Collect(t); (len(data.ScopeMetrics) > 0) != tc.metrics {
				t.Fatal("metrics toggle")
			}
		})
	}
	if NewClientHandler(nil) == nil || NewServerHandler(nil) == nil {
		t.Fatal("nil provider handlers")
	}
}
func (s *healthService) Watch(_ *grpc_health_v1.HealthCheckRequest, stream native.ServerStreamingServer[grpc_health_v1.HealthCheckResponse]) error {
	return stream.Send(&grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING})
}
func TestStreamingStatsLifecycle(t *testing.T) {
	p := testkit.New(t)
	conn := connect(t, p, &healthService{}, WithMessageEvents(ReceivedEvent, SentEvent))
	stream, err := grpc_health_v1.NewHealthClient(conn).Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(); err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Recv(); err != io.EOF {
		t.Fatal("native stream termination", err)
	}
	deadline := time.Now().Add(time.Second)
	for len(p.Recorder.Ended()) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(p.Recorder.Ended()) != 2 {
		t.Fatal("missing streaming client/server spans")
	}
	for _, span := range p.Recorder.Ended() {
		if len(span.Events()) == 0 {
			t.Fatal("message events not preserved")
		}
		for _, event := range span.Events() {
			if event.Name != "message" {
				t.Fatalf("unexpected event %q", event.Name)
			}
			for _, attr := range event.Attributes {
				if attr.Key != "rpc.message.type" && attr.Key != "rpc.message.id" && attr.Key != "rpc.message.compressed_size" && attr.Key != "rpc.message.uncompressed_size" {
					t.Fatalf("unsafe message attribute %q", attr.Key)
				}
			}
		}
	}
	p.AssertPrivate(t, "SECRET")
}
