package kafkago

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	kafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/codes"
	"testing"
)

func TestAsyncCompletionPreservesDeliveryAndMetadata(t *testing.T) {
	p := testkit.New(t)
	data := &struct {
		Label string `json:"label"`
	}{Label: "app"}
	callbackCalls := 0
	writer := &kafka.Writer{Async: true, Topic: "orders", Completion: func(messages []kafka.Message, err error) {
		callbackCalls++
		if err != nil || messages[0].WriterData != data {
			t.Errorf("completion changed: %v %v", messages, err)
		}
	}}
	w, err := WrapAsyncWriter(writer, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = WrapAsyncWriter(writer, p); err == nil {
		t.Fatal("duplicate wrapper accepted")
	}
	fake := &fakeWriter{}
	w.send = fake
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "request")
	message := kafka.Message{Value: []byte("SECRET"), WriterData: data, Headers: []kafka.Header{{Key: "app", Value: []byte("original")}}}
	if err = w.i.WriteMessages(ctx, w, message); err != nil {
		t.Fatal(err)
	}
	if len(p.Recorder.Ended()) != 0 {
		t.Fatal("span ended at enqueue instead of completion")
	}
	if len(message.Headers) != 1 || message.WriterData != data || len(fake.messages[0].Headers) < 2 {
		t.Fatal("caller state changed or propagation missing")
	}
	writer.Completion(fake.messages, nil)
	if callbackCalls != 1 || len(p.Recorder.Ended()) != 1 || p.Recorder.Ended()[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("completion/parentage")
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
}

func TestAsyncEnqueueErrorAndValidation(t *testing.T) {
	if _, err := WrapAsyncWriter(nil, nil); err == nil {
		t.Fatal("nil accepted")
	}
	if _, err := WrapAsyncWriter(&kafka.Writer{}, nil); err == nil {
		t.Fatal("delivery mode changed")
	}
	p := testkit.New(t)
	w, err := WrapAsyncWriter(&kafka.Writer{Async: true}, p)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("SECRET metadata failure")
	w.send = &fakeWriter{err: sentinel}
	if err = w.WriteMessages(context.Background(), kafka.Message{Topic: "orders"}); err != sentinel {
		t.Fatal("native error identity")
	}
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error || len(spans[0].Events()) != 1 {
		t.Fatal("failed enqueue telemetry")
	}
	p.AssertPrivate(t, "SECRET")
}
