package sarama

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	native "github.com/IBM/sarama"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type asyncFixture struct {
	native.AsyncProducer
	input     chan *native.ProducerMessage
	successes chan *native.ProducerMessage
	errors    chan *native.ProducerError
	closed    chan struct{}
	once      sync.Once
}

func newAsyncFixture() *asyncFixture {
	return &asyncFixture{input: make(chan *native.ProducerMessage), successes: make(chan *native.ProducerMessage), errors: make(chan *native.ProducerError), closed: make(chan struct{})}
}
func (f *asyncFixture) Input() chan<- *native.ProducerMessage     { return f.input }
func (f *asyncFixture) Successes() <-chan *native.ProducerMessage { return f.successes }
func (f *asyncFixture) Errors() <-chan *native.ProducerError      { return f.errors }
func (f *asyncFixture) AsyncClose() {
	f.once.Do(func() { close(f.closed); close(f.successes); close(f.errors) })
}
func (*asyncFixture) IsTransactional() bool { return true }
func asyncConfig() *native.Config {
	c := native.NewConfig()
	c.Producer.Return.Successes = true
	c.Producer.Return.Errors = true
	return c
}
func receiveMessage(t *testing.T, ch <-chan *native.ProducerMessage) *native.ProducerMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("producer did not progress")
		return nil
	}
}

func TestAsyncCompletionParentageMetadataAndSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			f := newAsyncFixture()
			cfg := asyncConfig()
			w, err := WrapAsyncProducer(f, cfg, p, WithTracing(signals[0]), WithMetrics(signals[1]))
			if err != nil {
				t.Fatal(err)
			}
			if !w.IsTransactional() || !cfg.Producer.Return.Successes {
				t.Fatal("native config/interface changed")
			}
			ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
			original := &native.ProducerMessage{Topic: "orders", Metadata: "application", Headers: []native.RecordHeader{{Key: []byte("app-header"), Value: []byte("kept")}}, Value: native.StringEncoder("SECRET")}
			if err = w.Send(ctx, original); err != nil {
				t.Fatal(err)
			}
			sent := receiveMessage(t, f.input)
			if len(p.Recorder.Ended()) != 0 {
				t.Fatal("span ended at enqueue")
			}
			if sent == original || sent.Metadata != "application" || len(original.Headers) != 1 || (HeaderCarrier{&sent.Headers}).Get("app-header") != "kept" {
				t.Fatal("metadata/header ownership changed")
			}
			if signals[0] {
				remote := p.TextMapPropagator().Extract(context.Background(), HeaderCarrier{&sent.Headers})
				if trace.SpanContextFromContext(remote).TraceID() != parent.SpanContext().TraceID() {
					t.Fatal("propagation")
				}
			}
			sent.Offset = 42
			sent.Partition = 2
			f.successes <- sent
			got := receiveMessage(t, w.Successes())
			if got != original || got.Offset != 42 || got.Partition != 2 || got.Metadata != "application" || len(got.Headers) != 1 {
				t.Fatal("native completion identity")
			}
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] {
				t.Fatal("signal mode")
			}
			if signals[0] && (spans[0].Parent().SpanID() != parent.SpanContext().SpanID() || spans[0].SpanKind() != trace.SpanKindProducer) {
				t.Fatal("producer span semantics")
			}
			if (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("metric signal mode")
			}
			parent.End()
			p.AssertPrivate(t, "SECRET")
			if err = w.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAsyncFailuresNativeInputAndClose(t *testing.T) {
	p := testkit.New(t)
	f := newAsyncFixture()
	w, err := WrapAsyncProducer(f, asyncConfig(), p)
	if err != nil {
		t.Fatal(err)
	}
	original := &native.ProducerMessage{Topic: "orders", Metadata: 17}
	w.Input() <- original
	sent := receiveMessage(t, f.input)
	failure := errors.New("SECRET delivery")
	f.errors <- &native.ProducerError{Msg: sent, Err: failure}
	got := <-w.Errors()
	if got.Msg != original || got.Err != failure || original.Metadata != 17 {
		t.Fatal("error identity")
	}
	if spans := p.Recorder.Ended(); len(spans) != 1 || spans[0].Status().Code != codes.Error || len(spans[0].Events()) != 1 {
		t.Fatal("error telemetry")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if w.Send(ctx, original) != context.Canceled || w.Send(context.Background(), nil) == nil {
		t.Fatal("invalid enqueue")
	}
	if _, err = WrapAsyncProducer(w, asyncConfig(), p); err == nil {
		t.Fatal("duplicate")
	}
	w.AsyncClose()
	if err = w.Send(context.Background(), original); err != native.ErrShuttingDown {
		t.Fatal("send after close")
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	p.AssertPrivate(t, "SECRET")

	f = newAsyncFixture()
	w, err = WrapAsyncProducer(f, asyncConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Send(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	sent = receiveMessage(t, f.input)
	f.errors <- &native.ProducerError{Msg: sent, Err: failure}
	err = w.Close()
	var failures native.ProducerErrors
	if !errors.As(err, &failures) || len(failures) != 1 || failures[0].Err != failure || failures[0].Msg != original {
		t.Fatal("Close must collect native delivery errors", err)
	}
	if _, err = WrapAsyncProducer(nil, asyncConfig(), nil); err == nil {
		t.Fatal("nil producer")
	}
	if _, err = WrapAsyncProducer(f, nil, nil); err == nil {
		t.Fatal("nil config")
	}
	if _, err = WrapAsyncProducer(f, native.NewConfig(), nil); err == nil {
		t.Fatal("missing completion channels")
	}
}

func TestAsyncShutdownClosesUnfinishedSpans(t *testing.T) {
	p := testkit.New(t)
	f := newAsyncFixture()
	w, err := WrapAsyncProducer(f, asyncConfig(), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Send(context.Background(), &native.ProducerMessage{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	_ = receiveMessage(t, f.input)
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if spans := p.Recorder.Ended(); len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatal("unexpected native shutdown left pending span")
	}
}
