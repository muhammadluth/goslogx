package kafkago

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	kafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

type fakeWriter struct {
	messages []kafka.Message
	err      error
}

func (w *fakeWriter) WriteMessages(_ context.Context, messages ...kafka.Message) error {
	w.messages = messages
	return w.err
}
func TestBatchAndProcess(t *testing.T) {
	p := testkit.New(t)
	instrument, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	writer := &fakeWriter{}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	message := kafka.Message{Topic: "topic.SECRET", Key: []byte("SECRET"), Value: []byte("SECRET")}
	if err := instrument.WriteMessages(ctx, writer, message); err != nil {
		t.Fatal(err)
	}
	if len(message.Headers) != 0 {
		t.Fatal("headers mutated")
	}
	if err := instrument.Process(context.Background(), writer.messages[0], func(ctx context.Context, msg kafka.Message) error {
		if trace.SpanContextFromContext(ctx).TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace continuity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	writer.err = kafka.WriteErrors{errors.New("SECRET")}
	if instrument.WriteMessages(ctx, writer, message) == nil {
		t.Fatal("error lost")
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}
