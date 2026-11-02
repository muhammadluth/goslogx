package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TestFetchWaitsForNativeBatchCompletion(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "error", false: "success"}[fail], func(t *testing.T) {
			p := fixture.New(t)
			j, err := WrapJetStream(&asyncPublisher{}, p)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("SECRET-native-error")
			in := make(chan jetstream.Msg, 2)
			native := &fixtureBatch{messages: in}
			if fail {
				native.err = failure
			}
			consumer := &fixtureConsumer{batch: native}
			ctx, parent := p.TP.Tracer("app").Start(context.Background(), "request")
			batch, err := j.Fetch(ctx, consumer, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Recorder.Ended()) != 0 {
				t.Fatal("span ended at establishment")
			}
			first := &fixtureMessage{}
			second := &fixtureMessage{}
			in <- first
			in <- second
			close(in)
			if batch.Messages() != batch.Messages() {
				t.Fatal("unstable channel")
			}
			for _, want := range []jetstream.Msg{first, second} {
				select {
				case got := <-batch.Messages():
					if got != want {
						t.Fatal("message identity/order")
					}
				case <-time.After(time.Second):
					t.Fatal("timeout")
				}
			}
			select {
			case _, ok := <-batch.Messages():
				if ok {
					t.Fatal("extra message")
				}
			case <-time.After(time.Second):
				t.Fatal("batch did not close")
			}
			if batch.Error() != native.err {
				t.Fatal("native error identity")
			}
			spans := p.Recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("spans=%d", len(spans))
			}
			s := spans[0]
			if s.SpanKind() != trace.SpanKindConsumer || s.Parent().SpanID() != parent.SpanContext().SpanID() || (s.Status().Code == codes.Error) != fail {
				t.Fatal("batch semantics")
			}
			p.AssertPrivate(t, "SECRET-native-error")
			parent.End()
		})
	}
}

func TestFetchImmediateFailure(t *testing.T) {
	p := fixture.New(t)
	j, err := WrapJetStream(&asyncPublisher{}, p)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("native failure")
	if _, err = j.Fetch(context.Background(), &fixtureConsumer{err: failure}, 1); err != failure {
		t.Fatal("error identity")
	}
	if len(p.Recorder.Ended()) != 1 {
		t.Fatal("span not ended")
	}
}
