package sarama

import (
	"context"
	"errors"
	"testing"

	native "github.com/IBM/sarama"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/codes"
)

type partialProducer struct {
	fakeProducer
	failure error
}

func (p *partialProducer) SendMessages(messages []*native.ProducerMessage) error {
	for i, msg := range messages {
		msg.Offset = int64(10 + i)
	}
	return native.ProducerErrors{&native.ProducerError{Msg: messages[1], Err: p.failure}}
}
func TestBatchErrorsRetainOriginalMessageIdentity(t *testing.T) {
	p := testkit.New(t)
	i, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("SECRET")
	producer := &partialProducer{failure: failure}
	messages := []*native.ProducerMessage{{Topic: "orders", Metadata: "first"}, {Topic: "orders", Metadata: "second"}}
	err = i.SendMessages(context.Background(), producer, messages)
	var failures native.ProducerErrors
	if !errors.As(err, &failures) || len(failures) != 1 || failures[0].Msg != messages[1] || failures[0].Err != failure {
		t.Fatal("batch error identity changed", err)
	}
	if messages[0].Offset != 10 || messages[1].Offset != 11 || messages[1].Metadata != "second" || len(messages[0].Headers) != 0 {
		t.Fatal("batch native assignments")
	}
	spans := p.Recorder.Ended()
	if len(spans) != 2 || spans[0].Status().Code == codes.Error || spans[1].Status().Code != codes.Error {
		t.Fatal("per-message failure attribution")
	}
	p.AssertPrivate(t, "SECRET")
}
func TestInvalidInputsAndGroupCancellation(t *testing.T) {
	i, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	producer := &fakeProducer{}
	if _, _, err = i.SendMessage(ctx, nil, &native.ProducerMessage{}); err == nil {
		t.Fatal("nil producer")
	}
	if _, _, err = i.SendMessage(ctx, producer, nil); err == nil {
		t.Fatal("nil message")
	}
	if i.SendMessages(ctx, nil, nil) == nil || i.SendMessages(ctx, producer, []*native.ProducerMessage{{}, nil}) == nil {
		t.Fatal("invalid batch")
	}
	if i.Process(ctx, nil, func(context.Context, *native.ConsumerMessage) error { return nil }) == nil || i.Process(ctx, &native.ConsumerMessage{}, nil) == nil {
		t.Fatal("invalid process")
	}
	if err = i.Process(ctx, &native.ConsumerMessage{Headers: []*native.RecordHeader{nil}}, func(context.Context, *native.ConsumerMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	h := i.GroupHandler(nil, nil, nil)
	session := &fakeSession{ctx: ctx}
	if h.Setup(session) != nil || h.Cleanup(session) != nil || h.ConsumeClaim(session, &fakeClaim{}) == nil {
		t.Fatal("group callbacks")
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	session.ctx = cancelCtx
	h = i.GroupHandler(nil, nil, func(context.Context, native.ConsumerGroupSession, *native.ConsumerMessage) error { return nil })
	if h.ConsumeClaim(session, &fakeClaim{messages: make(chan *native.ConsumerMessage)}) != context.Canceled {
		t.Fatal("group cancellation")
	}
	headers := []native.RecordHeader{{Key: []byte("app"), Value: []byte("value")}}
	if len((HeaderCarrier{&headers}).Keys()) != 1 {
		t.Fatal("header keys")
	}
}
