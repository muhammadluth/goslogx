package nats

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/nats-io/nats-server/v2/server"
	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func broker(t *testing.T) *gonats.Conn {
	t.Helper()
	s, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("broker not ready")
	}
	t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
	nc, err := gonats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}
func TestCoreAndJetStreamPropagation(t *testing.T) {
	nc := broker(t)
	p := testkit.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := Wrap(nc, p, WithDestinationNormalizer(func(s string) string { return "orders.{id}" }))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan trace.SpanContext, 1)
	sub, err := client.Subscribe(context.Background(), "orders.*", func(ctx context.Context, msg *gonats.Msg) error {
		if string(msg.Data) != "SECRET" {
			t.Error("payload changed")
		}
		done <- trace.SpanContextFromContext(ctx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	parentCtx, parent := p.TP.Tracer("application").Start(ctx, "request")
	original := &gonats.Msg{Subject: "orders.123SECRET", Data: []byte("SECRET")}
	if err := client.PublishMsg(parentCtx, original); err != nil {
		t.Fatal(err)
	}
	if len(original.Header) != 0 {
		t.Fatal("caller headers mutated")
	}
	select {
	case child := <-done:
		if child.TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace discontinuity")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"events.*"}}); err != nil {
		t.Fatal(err)
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, "ORDERS", jetstream.ConsumerConfig{Durable: "worker", AckPolicy: jetstream.AckExplicitPolicy})
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := WrapJetStream(js, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.Publish(parentCtx, "events.SECRET", []byte("SECRET")); err != nil {
		t.Fatal(err)
	}
	batch, err := wrapped.Fetch(ctx, consumer, 1, jetstream.FetchMaxWait(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for msg := range batch.Messages() {
		if err := wrapped.Process(ctx, msg, func(child context.Context, msg jetstream.Msg) error {
			if trace.SpanContextFromContext(child).TraceID() != parent.SpanContext().TraceID() {
				t.Fatal("JetStream discontinuity")
			}
			return msg.Ack()
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.Error(); err != nil {
		t.Fatal(err)
	}
	if info, err := consumer.Info(ctx); err == nil {
		wrapped.ObserveConsumer(ctx, info)
	} else {
		t.Fatal(err)
	}
	parent.End()
	p.AssertPrivate(t, "SECRET", "123SECRET")
	producer, process := false, false
	for _, s := range p.Recorder.Ended() {
		if s.InstrumentationScope().Name == ScopeName {
			if s.SpanKind() == trace.SpanKindProducer {
				producer = true
			}
			if strings.HasSuffix(s.Name(), " process") {
				process = true
			}
		}
	}
	if !producer || !process {
		t.Fatal("missing messaging spans")
	}
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("missing metrics")
	}
}

func TestRequestReplyBaggageAndDisabledSignals(t *testing.T) {
	nc := broker(t)
	p := testkit.New(t)
	p.Propagator = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	client, err := Wrap(nc, p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Subscribe(context.Background(), "rpc.orders", func(ctx context.Context, msg *gonats.Msg) error {
		if baggage.FromContext(ctx).Member("region").Value() != "test" {
			t.Error("baggage lost")
		}
		return client.Publish(ctx, msg.Reply, []byte("SECRET"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = nc.Flush(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, span := p.TP.Tracer("app").Start(ctx, "rpc")
	member, _ := baggage.NewMember("region", "test")
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)
	reply, err := client.Request(ctx, "rpc.orders", []byte("SECRET"))
	if err != nil {
		t.Fatal(err)
	}
	if string(reply.Data) != "SECRET" {
		t.Fatal("reply payload changed")
	}
	span.End()
	// Case-insensitive extraction tolerates headers from other language clients.
	headers := HeaderCarrier(gonats.Header{"TraceParent": []string{"value"}})
	if headers.Get("traceparent") != "value" {
		t.Fatal("header extraction")
	}
	headers.Set("traceparent", "new")
	if len(headers) != 1 || headers.Get("TRACEPARENT") != "new" {
		t.Fatal("header replacement")
	}
	p.AssertPrivate(t, "SECRET")
	off, err := Wrap(nc, p, WithTracing(false), WithMetrics(false))
	if err != nil {
		t.Fatal(err)
	}
	before := len(p.Recorder.Ended())
	if err = off.Process(ctx, &gonats.Msg{Header: reply.Header}, func(ctx context.Context, _ *gonats.Msg) error {
		if !trace.SpanContextFromContext(ctx).IsValid() {
			t.Error("disabled tracing lost propagation")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(p.Recorder.Ended()) != before {
		t.Fatal("disabled tracing emitted spans")
	}
}
