package nats

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type fixtureMessage struct {
	jetstream.Msg
	headers native.Header
}

func (m fixtureMessage) Subject() string        { return "orders" }
func (m fixtureMessage) Headers() native.Header { return m.headers }
func (fixtureMessage) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: 2}, nil
}

type fixtureConsumer struct {
	jetstream.Consumer
	callback jetstream.MessageHandler
	result   jetstream.ConsumeContext
	batch    jetstream.MessageBatch
	err      error
	size     int
}

func (c *fixtureConsumer) Consume(fn jetstream.MessageHandler, _ ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	c.callback = fn
	return c.result, c.err
}
func (c *fixtureConsumer) Fetch(size int, _ ...jetstream.FetchOpt) (jetstream.MessageBatch, error) {
	c.size = size
	return c.batch, c.err
}

type fixtureConsumeContext struct{ jetstream.ConsumeContext }
type fixtureBatch struct {
	messages chan jetstream.Msg
	err      error
}

func (b *fixtureBatch) Messages() <-chan jetstream.Msg { return b.messages }
func (b *fixtureBatch) Error() error                   { return b.err }

func TestQueueSubscriptionErrorsAndCancellation(t *testing.T) {
	nc := broker(t)
	p := testkit.New(t)
	reported := make(chan error, 1)
	failure := errors.New("SECRET")
	client, err := Wrap(nc, p, WithErrorHandler(func(err error) { reported <- err }))
	if err != nil {
		t.Fatal(err)
	}
	sub, err := client.QueueSubscribe(context.Background(), "orders", "workers", func(context.Context, *native.Msg) error { return failure })
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()
	if err = nc.Flush(); err != nil {
		t.Fatal(err)
	}
	msg := &native.Msg{Subject: "orders", Header: native.Header{"app": []string{"value"}}}
	if err = client.PublishMsg(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-reported:
		if err != failure {
			t.Fatal("callback error identity")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not complete")
	}
	if len(msg.Header) != 1 || msg.Header.Get("app") != "value" {
		t.Fatal("caller headers changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if client.Publish(ctx, "orders", nil) != context.Canceled {
		t.Fatal("canceled publish")
	}
	if _, err = client.RequestMsg(ctx, msg); !errors.Is(err, context.Canceled) {
		t.Fatal("native request cancellation", err)
	}
	if _, err = Wrap(nil, nil); err == nil {
		t.Fatal("nil connection")
	}
	if client.PublishMsg(ctx, nil) == nil || client.Process(ctx, nil, func(context.Context, *native.Msg) error { return nil }) == nil || client.Process(ctx, msg, nil) == nil {
		t.Fatal("invalid message/handler")
	}
	if _, err = client.RequestMsg(ctx, nil); err == nil {
		t.Fatal("nil request")
	}
	if _, err = client.Subscribe(ctx, "orders", nil); err == nil {
		t.Fatal("nil subscribe handler")
	}
	if _, err = client.QueueSubscribe(ctx, "orders", "workers", nil); err == nil {
		t.Fatal("nil queue handler")
	}
	carrier := HeaderCarrier(msg.Header)
	if carrier.Get("missing") != "" || len(carrier.Keys()) != 1 {
		t.Fatal("carrier")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestJetStreamCompletionRedeliveryAndConsumerSnapshot(t *testing.T) {
	nc := broker(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	p := testkit.New(t)
	reported := make(chan error, 1)
	j, err := WrapJetStream(js, p, WithConsumerName("orders-worker"), WithMetricAttributes(attribute.String("app.region", "test")), WithErrorHandler(func(err error) { reported <- err }))
	if err != nil {
		t.Fatal(err)
	}
	closedMessages := make(chan jetstream.Msg)
	close(closedMessages)
	consumer := &fixtureConsumer{result: &fixtureConsumeContext{}, batch: &fixtureBatch{messages: closedMessages}}
	failure := errors.New("SECRET")
	result, err := j.Consume(nil, consumer, func(context.Context, jetstream.Msg) error { return failure })
	if err != nil || result != consumer.result {
		t.Fatal("native consume context")
	}
	consumer.callback(fixtureMessage{})
	if err = <-reported; err != failure {
		t.Fatal("JetStream callback error")
	}
	batch, err := j.Fetch(context.Background(), consumer, 7)
	if err != nil || batch == nil || consumer.size != 7 {
		t.Fatal("fetch contract")
	}
	for range batch.Messages() {
	}
	j.ObserveConsumer(nil, &jetstream.ConsumerInfo{NumPending: 5, NumAckPending: 2})
	j.ObserveConsumer(nil, nil)
	data := p.Collect(t)
	redeliveries := int64(0)
	lag := int64(0)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "messaging.nats.redelivered.messages" {
				for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
					redeliveries += dp.Value
				}
			}
			if m.Name == "messaging.nats.consumer.lag.messages" {
				for _, dp := range m.Data.(metricdata.Gauge[int64]).DataPoints {
					lag = dp.Value
				}
			}
		}
	}
	if redeliveries != 1 || lag != 7 || !strings.Contains(fmt.Sprint(data), "orders-worker") || !strings.Contains(fmt.Sprint(data), "app.region") {
		t.Fatal("native metadata measurements", data)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = j.Fetch(ctx, consumer, 1); err != context.Canceled {
		t.Fatal("canceled fetch")
	}
	if _, err = WrapJetStream(nil, nil); err == nil {
		t.Fatal("nil JetStream")
	}
	if _, err = j.PublishMsg(ctx, nil); err == nil {
		t.Fatal("nil publish")
	}
	if j.Process(ctx, nil, func(context.Context, jetstream.Msg) error { return nil }) == nil || j.Process(ctx, fixtureMessage{}, nil) == nil {
		t.Fatal("invalid process")
	}
	if _, err = j.Consume(ctx, nil, func(context.Context, jetstream.Msg) error { return nil }); err == nil {
		t.Fatal("nil consumer")
	}
	if _, err = j.Consume(ctx, consumer, nil); err == nil {
		t.Fatal("nil handler")
	}
	if _, err = j.Fetch(ctx, nil, 1); err == nil {
		t.Fatal("nil fetch consumer")
	}
	p.AssertPrivate(t, "SECRET")
}
