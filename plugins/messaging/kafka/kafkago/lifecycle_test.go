package kafkago

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	kafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/codes"
)

type fakeReader struct {
	message kafka.Message
	err     error
}

func (r fakeReader) FetchMessage(context.Context) (kafka.Message, error) { return r.message, r.err }

func TestReaderBatchFailuresAndIndependentSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			i, err := New(p, WithTracing(signals[0]), WithMetrics(signals[1]))
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("SECRET")
			writer := &fakeWriter{err: kafka.WriteErrors{nil, failure}}
			ctx := context.Background()
			messages := []kafka.Message{{Topic: "orders"}, {Topic: "orders"}}
			if err = i.WriteMessages(ctx, writer, messages...); err != nil {
				var failures kafka.WriteErrors
				if !errors.As(err, &failures) || failures[1] != failure {
					t.Fatal("batch failure identity")
				}
			} else {
				t.Fatal("lost batch error")
			}
			if signals[0] {
				spans := p.Recorder.Ended()
				if len(spans) != 2 || spans[0].Status().Code == codes.Error || spans[1].Status().Code != codes.Error {
					t.Fatal("per-message error status")
				}
			}
			message, err := i.FetchMessage(ctx, fakeReader{message: kafka.Message{Topic: "orders", Offset: 42}})
			if err != nil || message.Offset != 42 {
				t.Fatal("reader result")
			}
			if _, err = i.FetchMessage(ctx, fakeReader{err: failure}); err != failure {
				t.Fatal("reader failure")
			}
			calls := 0
			if err = i.ProcessBatch(ctx, messages, func(context.Context, kafka.Message) error { calls++; return failure }); !errors.Is(err, failure) || calls != 2 {
				t.Fatal("batch process semantics")
			}
			if (len(p.Recorder.Ended()) > 0) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("signal modes")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	i, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if i.WriteMessages(ctx, nil) == nil || i.WriteMessages(ctx, &kafka.Writer{Async: true}, kafka.Message{}) == nil || i.Process(ctx, kafka.Message{}, nil) == nil {
		t.Fatal("invalid writer/handler")
	}
	if _, err = i.FetchMessage(ctx, nil); err == nil {
		t.Fatal("nil reader")
	}
	if err = i.WriteMessages(ctx, &kafka.Writer{Topic: "orders"}, kafka.Message{}); err == nil {
		t.Fatal("native missing-address error")
	}
	headers := []kafka.Header{{Key: "TraceParent", Value: []byte("old")}, {Key: "app", Value: []byte("kept")}}
	carrier := HeaderCarrier{&headers}
	carrier.Set("traceparent", "new")
	if len(carrier.Keys()) != 2 || carrier.Get("TRACEPARENT") != "new" || carrier.Get("missing") != "" {
		t.Fatal("carrier behavior")
	}
}
