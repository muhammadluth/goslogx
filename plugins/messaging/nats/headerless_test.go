package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/nats-io/nats-server/v2/server"
	native "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/trace"
)

func TestHeaderlessServerKeepsPublishAndRequestWorking(t *testing.T) {
	s, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoHeaderSupport: true, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("server not ready")
	}
	defer func() { s.Shutdown(); s.WaitForShutdown() }()
	nc, err := native.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	if nc.HeadersSupported() {
		t.Fatal("fixture advertised headers")
	}
	p := testkit.New(t)
	client, err := Wrap(nc, p)
	if err != nil {
		t.Fatal(err)
	}
	delivered := make(chan trace.SpanContext, 1)
	sub, err := client.Subscribe(context.Background(), "orders", func(ctx context.Context, msg *native.Msg) error {
		if string(msg.Data) != "SECRET" {
			t.Error("payload changed")
		}
		delivered <- trace.SpanContextFromContext(ctx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	rpc, err := nc.Subscribe("rpc", func(msg *native.Msg) { _ = msg.Respond([]byte("ok")) })
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.Unsubscribe()
	if err = nc.Flush(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx, parent := p.TP.Tracer("app").Start(ctx, "request")
	defer parent.End()
	if err = client.Publish(ctx, "orders", []byte("SECRET")); err != nil {
		t.Fatal("instrumentation broke headerless publish", err)
	}
	select {
	case remote := <-delivered:
		if remote.TraceID() == parent.SpanContext().TraceID() {
			t.Fatal("fabricated remote parent without transport headers")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	reply, err := client.Request(ctx, "rpc", []byte("SECRET"))
	if err != nil || string(reply.Data) != "ok" {
		t.Fatal("instrumentation broke headerless request", err)
	}
	withHeaders := &native.Msg{Subject: "orders", Header: native.Header{"app-header": []string{"keep"}}}
	if err = client.PublishMsg(ctx, withHeaders); !errors.Is(err, native.ErrHeadersNotSupported) {
		t.Fatal("application headers must retain native error", err)
	}
	if withHeaders.Header.Get("app-header") != "keep" {
		t.Fatal("caller headers changed")
	}
	p.AssertPrivate(t, "SECRET")
}
