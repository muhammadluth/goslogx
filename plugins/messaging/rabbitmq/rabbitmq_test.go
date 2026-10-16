package rabbitmq

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

type fakeChannel struct {
	message amqp.Publishing
	err     error
}

func (c *fakeChannel) PublishWithContext(_ context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	c.message = msg
	return c.err
}
func (c *fakeChannel) ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error) {
	return make(chan amqp.Delivery), nil
}

type fakeAck struct{ acked, nacked, rejected bool }

func (a *fakeAck) Ack(uint64, bool) error        { a.acked = true; return nil }
func (a *fakeAck) Nack(uint64, bool, bool) error { a.nacked = true; return nil }
func (a *fakeAck) Reject(uint64, bool) error     { a.rejected = true; return nil }
func TestPublishProcessSettlementPrivacy(t *testing.T) {
	p := testkit.New(t)
	native := &fakeChannel{}
	client, err := WrapChannel(native, p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	msg := amqp.Publishing{Body: []byte("SECRET"), Headers: amqp.Table{"authorization": "SECRET"}, CorrelationId: "SECRET", ReplyTo: "SECRET"}
	if err := client.Publish(ctx, "exchange.SECRET", "key.SECRET", true, false, msg); err != nil {
		t.Fatal(err)
	}
	if len(msg.Headers) != 1 || native.message.CorrelationId != msg.CorrelationId || native.message.ReplyTo != msg.ReplyTo {
		t.Fatal("native message semantics changed")
	}
	ack := &fakeAck{}
	delivery := amqp.Delivery{Headers: native.message.Headers, Body: msg.Body, Acknowledger: ack, DeliveryTag: 1, Redelivered: true}
	if err := client.Process(context.Background(), "queue.SECRET", delivery, func(ctx context.Context, d amqp.Delivery) error {
		if trace.SpanContextFromContext(ctx).TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace continuity")
		}
		return d.Ack(false)
	}); err != nil {
		t.Fatal(err)
	}
	if !ack.acked {
		t.Fatal("ack lost")
	}
	if err := client.Nack(ctx, "queue.SECRET", delivery, false, true); err != nil {
		t.Fatal(err)
	}
	if err := client.Reject(ctx, "queue.SECRET", delivery, true); err != nil {
		t.Fatal(err)
	}
	native.err = errors.New("SECRET broker error")
	if client.Publish(ctx, "exchange.SECRET", "key.SECRET", false, false, msg) != native.err {
		t.Fatal("error changed")
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("missing metrics")
	}
}
