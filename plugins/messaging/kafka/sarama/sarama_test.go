package sarama

import (
	"context"
	"errors"
	native "github.com/IBM/sarama"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

type fakeProducer struct{ messages []*native.ProducerMessage }

func (p *fakeProducer) SendMessage(msg *native.ProducerMessage) (int32, int64, error) {
	p.messages = append(p.messages, msg)
	msg.Offset = 42
	return 0, 42, nil
}
func (p *fakeProducer) SendMessages(messages []*native.ProducerMessage) error {
	p.messages = messages
	return nil
}
func TestModernSaramaPropagation(t *testing.T) {
	p := testkit.New(t)
	instrument, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	producer := &fakeProducer{}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	msg := &native.ProducerMessage{Topic: "topic.SECRET", Key: native.StringEncoder("SECRET"), Value: native.StringEncoder("SECRET")}
	if _, _, err := instrument.SendMessage(ctx, producer, msg); err != nil {
		t.Fatal(err)
	}
	headers := producer.messages[0].Headers
	consumer := &native.ConsumerMessage{Topic: msg.Topic}
	for index := range headers {
		consumer.Headers = append(consumer.Headers, &headers[index])
	}
	if err := instrument.Process(context.Background(), consumer, func(ctx context.Context, msg *native.ConsumerMessage) error {
		if trace.SpanContextFromContext(ctx).TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace continuity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(msg.Headers) != 0 || msg.Offset != 42 {
		t.Fatal("producer metadata changed")
	}
	if err := instrument.SendMessages(ctx, producer, []*native.ProducerMessage{msg}); err != nil {
		t.Fatal(err)
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}

type fakeSession struct {
	native.ConsumerGroupSession
	ctx    context.Context
	marked int
}

func (s *fakeSession) Context() context.Context                    { return s.ctx }
func (s *fakeSession) MarkMessage(*native.ConsumerMessage, string) { s.marked++ }

type fakeClaim struct {
	native.ConsumerGroupClaim
	messages chan *native.ConsumerMessage
}

func (c *fakeClaim) Messages() <-chan *native.ConsumerMessage { return c.messages }
func TestGroupCommitPolicyAndErrors(t *testing.T) {
	p := testkit.New(t)
	i, err := New(p, WithConsumerGroup("orders-worker"))
	if err != nil {
		t.Fatal(err)
	}
	session := &fakeSession{ctx: context.Background()}
	claim := &fakeClaim{messages: make(chan *native.ConsumerMessage, 2)}
	claim.messages <- &native.ConsumerMessage{Topic: "SECRET"}
	close(claim.messages)
	setup, cleanup := false, false
	handler := i.GroupHandler(func(native.ConsumerGroupSession) error { setup = true; return nil }, func(native.ConsumerGroupSession) error { cleanup = true; return nil }, func(ctx context.Context, s native.ConsumerGroupSession, msg *native.ConsumerMessage) error {
		if !trace.SpanContextFromContext(ctx).IsValid() {
			t.Error("missing processing context")
		}
		s.MarkMessage(msg, "")
		return nil
	})
	if err = handler.Setup(session); err != nil {
		t.Fatal(err)
	}
	if err = handler.ConsumeClaim(session, claim); err != nil {
		t.Fatal(err)
	}
	if err = handler.Cleanup(session); err != nil {
		t.Fatal(err)
	}
	if !setup || !cleanup || session.marked != 1 {
		t.Fatal("group lifecycle or commit policy changed")
	}
	claim.messages = make(chan *native.ConsumerMessage, 1)
	claim.messages <- &native.ConsumerMessage{Topic: "SECRET"}
	close(claim.messages)
	sentinel := errors.New("SECRET")
	handler = i.GroupHandler(nil, nil, func(context.Context, native.ConsumerGroupSession, *native.ConsumerMessage) error { return sentinel })
	if err = handler.ConsumeClaim(session, claim); err != sentinel {
		t.Fatal("handler error changed")
	}
	if session.marked != 1 {
		t.Fatal("automatically marked failed message")
	}
	p.AssertPrivate(t, "SECRET")
}
