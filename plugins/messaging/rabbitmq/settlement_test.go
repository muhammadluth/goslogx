package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/codes"
)

type recordingAck struct {
	calls             int
	tag               uint64
	multiple, requeue bool
	err               error
}

func (a *recordingAck) Ack(tag uint64, multiple bool) error {
	a.calls++
	a.tag = tag
	a.multiple = multiple
	return a.err
}
func (a *recordingAck) Nack(tag uint64, multiple, requeue bool) error {
	a.calls++
	a.tag = tag
	a.multiple = multiple
	a.requeue = requeue
	return a.err
}
func (a *recordingAck) Reject(tag uint64, requeue bool) error {
	a.calls++
	a.tag = tag
	a.requeue = requeue
	return a.err
}

type consumeChannel struct {
	fakeChannel
	deliveries                          chan amqp.Delivery
	args                                amqp.Table
	autoAck, exclusive, noLocal, noWait bool
	err                                 error
}

func (c *consumeChannel) ConsumeWithContext(_ context.Context, _, _ string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	c.autoAck, c.exclusive, c.noLocal, c.noWait = autoAck, exclusive, noLocal, noWait
	c.args = args
	return c.deliveries, c.err
}

func TestSettlementIdentityFlagsAndNoDuplicateSpans(t *testing.T) {
	for _, op := range []string{"ack", "nack", "reject"} {
		for _, failure := range []error{nil, errors.New("SECRET settlement")} {
			t.Run(fmt.Sprint(op, failure == nil), func(t *testing.T) {
				p := testkit.New(t)
				client, err := WrapChannel(&fakeChannel{}, p)
				if err != nil {
					t.Fatal(err)
				}
				ack := &recordingAck{err: failure}
				delivery := amqp.Delivery{Acknowledger: ack, DeliveryTag: 42}
				err = client.Process(nil, "orders", delivery, func(ctx context.Context, d amqp.Delivery) error {
					switch op {
					case "ack":
						return client.Ack(ctx, "orders", d, true)
					case "nack":
						return client.Nack(ctx, "orders", d, true, true)
					default:
						return client.Reject(ctx, "orders", d, true)
					}
				})
				if err != failure || ack.calls != 1 || ack.tag != 42 || (op != "ack" && !ack.requeue) || (op != "reject" && !ack.multiple) {
					t.Fatal("acknowledgement semantics changed")
				}
				spans := p.Recorder.Ended()
				if len(spans) != 2 || spans[0].Parent().SpanID() != spans[1].SpanContext().SpanID() {
					t.Fatal("duplicate settlement or wrong parent")
				}
				if (spans[0].Status().Code == codes.Error) != (failure != nil) {
					t.Fatal("settlement error")
				}
				p.AssertPrivate(t, "SECRET")
			})
		}
	}
}

func TestNativeConsumeAndIndependentSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			native := &consumeChannel{deliveries: make(chan amqp.Delivery, 1)}
			client, err := WrapChannel(native, p, WithTracing(signals[0]), WithMetrics(signals[1]), WithRoutingKeyNormalizer(func(string) string { return "orders.*" }))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			args := amqp.Table{"x-priority": 5}
			stream, err := client.Consume(ctx, "orders", "worker", false, true, true, false, args)
			if err != nil || stream != native.deliveries || native.autoAck || !native.exclusive || !native.noLocal || native.noWait || native.args["x-priority"] != 5 {
				t.Fatal("native consume contract")
			}
			if err = client.Publish(ctx, "orders", "orders.SECRET", false, false, amqp.Publishing{}); err != nil {
				t.Fatal(err)
			}
			ack := &recordingAck{}
			if err = client.Ack(ctx, "orders", amqp.Delivery{Acknowledger: ack, DeliveryTag: 7}, false); err != nil || ack.calls != 1 {
				t.Fatal("external acknowledgement")
			}
			if err = client.Process(ctx, "orders", amqp.Delivery{}, func(context.Context, amqp.Delivery) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if (len(p.Recorder.Ended()) > 0) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("signal modes")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	if _, err := WrapChannel(nil, nil); err == nil {
		t.Fatal("nil channel")
	}
	client, err := WrapChannel(&fakeChannel{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, err := range []error{client.Process(ctx, "", amqp.Delivery{}, nil), client.Ack(ctx, "", amqp.Delivery{}, false), client.Nack(ctx, "", amqp.Delivery{}, false, false), client.Reject(ctx, "", amqp.Delivery{}, false)} {
		if err == nil {
			t.Fatal("missing acknowledger/handler")
		}
	}
	headers := HeaderCarrier{"TraceParent": []byte("original"), "number": 1}
	if headers.Get("traceparent") != "original" || headers.Get("number") != "" || len(headers.Keys()) != 2 {
		t.Fatal("header carrier")
	}
	headers.Set("traceparent", "updated")
	if len(headers) != 2 || headers.Get("TRACEPARENT") != "updated" {
		t.Fatal("header replacement")
	}
}
