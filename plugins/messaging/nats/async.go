package nats

import (
	"context"
	"errors"

	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/trace"
)

// publishFuture forwards native completion without consuming the application's
// result. Like the native future, each channel has one buffered result and stays
// open. Msg delegates to the native future's published message.
type publishFuture struct {
	jetstream.PubAckFuture
	ok  chan *jetstream.PubAck
	err chan error
}

func (f *publishFuture) Ok() <-chan *jetstream.PubAck { return f.ok }
func (f *publishFuture) Err() <-chan error            { return f.err }

// PublishAsync retains the native API. Use PublishMsgAsyncContext to supply a
// parent context. Completion, retries and timeouts remain owned by JetStream.
func (j *JetStream) PublishAsync(subject string, data []byte, opts ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	return j.PublishMsgAsync(&gonats.Msg{Subject: subject, Data: data}, opts...)
}

func (j *JetStream) PublishMsgAsync(msg *gonats.Msg, opts ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	return j.PublishMsgAsyncContext(context.Background(), msg, opts...)
}

// PublishMsgAsyncContext traces enqueue through native acknowledgement/failure.
// Cancellation after enqueue does not cancel delivery or end the span early.
// Call native CleanupPublisher on shutdown to resolve outstanding futures before
// flushing telemetry. No payload bytes or subject are added as metric labels.
func (j *JetStream) PublishMsgAsyncContext(ctx context.Context, msg *gonats.Msg, opts ...jetstream.PublishOpt) (jetstream.PubAckFuture, error) {
	if msg == nil {
		return nil, errors.New("nats instrumentation: nil message")
	}
	ctx, finish := j.t.Begin(ctx, "publish", msg.Subject, trace.SpanKindProducer)
	copy := clone(msg)
	j.t.Inject(ctx, HeaderCarrier(copy.Header))
	future, err := j.JetStream.PublishMsgAsync(copy, opts...)
	if err != nil {
		finish(err)
		return future, err
	}
	f := &publishFuture{PubAckFuture: future, ok: make(chan *jetstream.PubAck, 1), err: make(chan error, 1)}
	go func() {
		select {
		case ack := <-future.Ok():
			finish(nil)
			f.ok <- ack
		case err := <-future.Err():
			finish(err)
			f.err <- err
		}
	}()
	return f, nil
}
