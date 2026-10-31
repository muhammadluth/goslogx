package nats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	fixture "github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type asyncPublisher struct {
	jetstream.JetStream
	future       *publishFuture
	enqueueError error
	message      *native.Msg
	options      int
}

func (p *asyncPublisher) PublishMsgAsync(m *native.Msg, opts ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	p.message = m
	p.options = len(opts)
	return p.future, p.enqueueError
}

func TestAsyncPublishCompletion(t *testing.T) {
	for _, tracing := range []bool{false, true} {
		for _, metrics := range []bool{false, true} {
			for _, failed := range []bool{false, true} {
				t.Run(fmt.Sprintf("trace=%v/metrics=%v/failure=%v", tracing, metrics, failed), func(t *testing.T) {
					p := fixture.New(t)
					upstream := &asyncPublisher{future: &publishFuture{ok: make(chan *jetstream.PubAck, 1), err: make(chan error, 1)}}
					j, err := WrapJetStream(upstream, p, WithTracing(tracing), WithMetrics(metrics))
					if err != nil {
						t.Fatal(err)
					}
					parentCtx, parent := p.TP.Tracer("app").Start(context.Background(), "request")
					ctx, cancel := context.WithCancel(parentCtx)
					defer cancel()
					msg := &native.Msg{Subject: "SECRET-subject", Data: []byte("SECRET-payload"), Header: native.Header{"App": []string{"preserved"}}}
					future, err := j.PublishMsgAsyncContext(ctx, msg, jetstream.WithMsgID("id"))
					if err != nil {
						t.Fatal(err)
					}
					cancel()
					if len(p.Recorder.Ended()) != 0 {
						t.Fatal("ended before native acknowledgement")
					}
					if upstream.message == msg || msg.Header.Get("traceparent") != "" || upstream.message.Header.Get("App") != "preserved" || upstream.options != 1 {
						t.Fatal("message/options changed")
					}
					if upstream.message.Header.Get("traceparent") == "" {
						t.Fatal("missing propagation")
					}
					failure := errors.New("SECRET-error")
					ack := &jetstream.PubAck{Stream: "orders", Sequence: 1}
					if failed {
						upstream.future.err <- failure
						select {
						case got := <-future.Err():
							if got != failure {
								t.Fatal("error identity")
							}
						case <-time.After(time.Second):
							t.Fatal("completion timeout")
						}
					} else {
						upstream.future.ok <- ack
						select {
						case got := <-future.Ok():
							if got != ack {
								t.Fatal("ack identity")
							}
						case <-time.After(time.Second):
							t.Fatal("completion timeout")
						}
					}
					spans := p.Recorder.Ended()
					want := 0
					if tracing {
						want = 1
					}
					if len(spans) != want {
						t.Fatalf("spans=%d", len(spans))
					}
					if tracing {
						s := spans[0]
						if s.SpanKind() != trace.SpanKindProducer || s.Parent().SpanID() != parent.SpanContext().SpanID() {
							t.Fatal("parent/kind")
						}
						if (s.Status().Code == codes.Error) != failed {
							t.Fatal("status")
						}
					}
					data := p.Collect(t)
					if metrics && len(data.ScopeMetrics) == 0 {
						t.Fatal("missing metrics")
					}
					if !metrics && len(data.ScopeMetrics) != 0 {
						t.Fatal("unexpected metrics")
					}
					p.AssertPrivate(t, "SECRET-payload", "SECRET-error", "SECRET-subject")
					parent.End()
				})
			}
		}
	}
}

func TestAsyncPublishEnqueueFailureAndNativeMethods(t *testing.T) {
	p := fixture.New(t)
	failure := errors.New("enqueue failed")
	upstream := &asyncPublisher{enqueueError: failure}
	j, err := WrapJetStream(upstream, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.PublishMsgAsync(nil); err == nil {
		t.Fatal("nil message")
	}
	if _, err = j.PublishAsync("orders", []byte("payload")); err != failure {
		t.Fatal("enqueue error identity")
	}
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatal("enqueue failure telemetry")
	}
}

func TestAsyncPublishNativeBroker(t *testing.T) {
	nc := broker(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	defer js.CleanupPublisher()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "orders", Subjects: []string{"orders"}, Storage: jetstream.MemoryStorage}); err != nil {
		t.Fatal(err)
	}
	p := fixture.New(t)
	observed, err := WrapJetStream(js, p)
	if err != nil {
		t.Fatal(err)
	}
	future, err := observed.PublishAsync("orders", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ack := <-future.Ok():
		if ack.Stream != "orders" || ack.Sequence != 1 {
			t.Fatalf("ack=%v", ack)
		}
	case err := <-future.Err():
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if future.Msg().Subject != "orders" || string(future.Msg().Data) != "payload" {
		t.Fatal("native future message changed")
	}
	if len(p.Recorder.Ended()) != 1 {
		t.Fatal("missing completion span")
	}
	if observed.PublishAsyncPending() != 0 {
		t.Fatal("native pending state changed")
	}
	select {
	case <-observed.PublishAsyncComplete():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
