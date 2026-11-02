package privacy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestErrorAndEventSemantics(t *testing.T) {
	r := tracetest.NewSpanRecorder()
	p := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r))
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	_, s := p.Tracer("upstream").Start(context.Background(), "operation")
	at := time.Now().Add(-time.Second)
	RecordError(s, nil)
	RecordError(s, fmt.Errorf("credential SECRET: %w", context.DeadlineExceeded), trace.WithTimestamp(at), trace.WithAttributes(attribute.Bool("exception.escaped", true), attribute.String("exception.stacktrace", "SECRET")))
	Event(s, "message", trace.WithTimestamp(at), trace.WithAttributes(attribute.Int("rpc.message.id", 3), attribute.String("rpc.message.payload", "SECRET")))
	Event(s, "SECRET", trace.WithAttributes(attribute.String("app.phase", "decode")))
	Event(s, "")
	Event(s, strings.Repeat("a", 129))
	s.End()
	got := r.Ended()[0]
	if len(got.Events()) != 3 || got.Events()[0].Name != "exception" || !got.Events()[0].Time.Equal(at) || got.Events()[1].Name != "message" || got.Events()[2].Name != "event" {
		t.Fatalf("events: %v", got.Events())
	}
	if text := fmt.Sprint(got.Events(), got.Attributes()); strings.Contains(text, "SECRET") || !strings.Contains(text, "context.DeadlineExceeded") || !strings.Contains(text, "rpc.message.id") {
		t.Fatal(text)
	}
	for _, tc := range []struct {
		err  error
		want string
	}{{nil, ""}, {context.Canceled, "context.Canceled"}, {context.DeadlineExceeded, "context.DeadlineExceeded"}, {errors.New("SECRET"), "*errors.errorString"}} {
		if ErrorType(tc.err) != tc.want {
			t.Fatalf("classification: %s", ErrorType(tc.err))
		}
	}
	for _, text := range []string{"context canceled", "context deadline exceeded", "EOF", "unexpected EOF"} {
		if StatusDescription(codes.Error, text) != text {
			t.Fatal(text)
		}
	}
	if StatusDescription(codes.Error, "SECRET") != "operation failed" || StatusDescription(codes.Ok, "SECRET") != "" {
		t.Fatal("status privacy")
	}
}

func TestMetadataLinksAndConnectionStrings(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "redis"), attribute.String("db.operation.name", "get"),
		attribute.String("server.address", "cache.internal"), attribute.Int("server.port", 6379),
		attribute.String("aws.region", "eu-west-1"), attribute.String("app.component", "cache"),
		attribute.String("db.statement", "SECRET"), attribute.String("authorization", "SECRET"),
		attribute.String("unknown", "SECRET"), attribute.String("app.too_long", strings.Repeat("x", 4097)),
	}
	context := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, Remote: true})
	link := Link(trace.Link{SpanContext: context, Attributes: attrs})
	if !link.SpanContext.Equal(context) || len(link.Attributes) != 6 || strings.Contains(fmt.Sprint(link.Attributes), "SECRET") {
		t.Fatalf("link: %v", link)
	}
	if ConnectionString("redis://user:SECRET@cache.internal:6379/0?token=SECRET") != "redis://cache.internal:6379" {
		t.Fatal("endpoint sanitization")
	}
	for _, invalid := range []string{"postgres user=SECRET", "unix:///SECRET", "%", "redis://"} {
		if ConnectionString(invalid) != "" {
			t.Fatal(invalid)
		}
	}
}

func TestNativeErrorLabels(t *testing.T) {
	for _, value := range []string{"404", "503", "_OTHER", "Canceled", "Unavailable", "net.OpError", "*net.OpError", "example.org/client.Error"} {
		if got := ErrorLabel(value, "fallback"); got != value {
			t.Fatal("lost native error classification", got, value)
		}
	}
	for _, value := range []string{"SECRET", "999", "5x3", "59x", "url.Error: SECRET", strings.Repeat("x", 257) + ".Error"} {
		if got := ErrorLabel(value, "fallback"); got != "fallback" {
			t.Fatal("raw error text accepted", got)
		}
	}
}

func TestEventNamePolicyPreservesUpstreamOnly(t *testing.T) {
	r := tracetest.NewSpanRecorder()
	p := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r))
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	_, s := p.Tracer("native").Start(context.Background(), "operation")

	upstream := []string{"exception", "message", "read", "write", "sql.rows.next",
		"db.connection.create", "db.connection.wait", "db.connection.closed"}
	for _, name := range upstream {
		Event(s, name)
	}
	// Application-controlled names are not part of the upstream surface and are
	// replaced, including well-formed identifiers, to avoid exporting raw text.
	for _, name := range []string{"SECRET", "cache.miss", "retry.attempt", "queue.process"} {
		Event(s, name)
	}
	s.End()

	events := r.Ended()[0].Events()
	if len(events) != len(upstream)+4 {
		t.Fatalf("unexpected event count: %d", len(events))
	}
	for i, name := range upstream {
		if events[i].Name != name {
			t.Fatalf("upstream event %q changed to %q", name, events[i].Name)
		}
	}
	for i := len(upstream); i < len(events); i++ {
		if events[i].Name != "event" {
			t.Fatalf("application event %q leaked", events[i].Name)
		}
	}
}

func TestPinnedUpstreamEventInventory(t *testing.T) {
	r := tracetest.NewSpanRecorder()
	p := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r))
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })
	_, s := p.Tracer("native").Start(context.Background(), "operation")
	at := time.Now().Add(-time.Second)
	names := []string{"message", "read", "write", "sql.rows.next", "exception", "db.connection.create", "db.connection.wait", "db.connection.closed"}
	for _, name := range names {
		Event(s, name, trace.WithTimestamp(at), trace.WithAttributes(attribute.Int("rpc.message.id", 1), attribute.String("exception.message", "SECRET")))
	}
	s.End()
	events := r.Ended()[0].Events()
	if len(events) != len(names) {
		t.Fatal("events dropped")
	}
	for i, e := range events {
		if e.Name != names[i] || !e.Time.Equal(at) || len(e.Attributes) != 1 {
			t.Fatalf("native event changed: %+v", e)
		}
	}
}
