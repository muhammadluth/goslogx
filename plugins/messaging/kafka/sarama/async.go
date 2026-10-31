package sarama

import (
	"context"
	"errors"
	"sync"

	native "github.com/IBM/sarama"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/trace"
)

// AsyncProducer preserves Sarama's transaction methods and completion channels.
// Configure it before first use and use only the wrapper thereafter. As with
// Sarama, drain both output channels when using AsyncClose.
type AsyncProducer struct {
	native.AsyncProducer
	i            *Instrumentation
	input        chan *native.ProducerMessage
	requests     chan asyncRequest
	transactions chan func()
	successes    chan *native.ProducerMessage
	errors       chan *native.ProducerError
	stop         chan struct{}
	once         sync.Once
	mu           sync.Mutex
	pending      map[*native.ProducerMessage]asyncCompletion
}
type asyncRequest struct {
	ctx     context.Context
	message *native.ProducerMessage
}
type asyncCompletion struct {
	original *native.ProducerMessage
	finish   func(error)
}

// WrapAsyncProducer requires the same config used to construct producer, with
// Return.Successes and Return.Errors enabled. It never changes delivery settings,
// replaces application Metadata, or changes transaction ownership.
func WrapAsyncProducer(producer native.AsyncProducer, cfg *native.Config, p goslogx.TelemetryProvider, opts ...Option) (*AsyncProducer, error) {
	if telemetry.Nil(producer) || cfg == nil {
		return nil, errors.New("sarama: nil async producer or config")
	}
	if _, ok := producer.(*AsyncProducer); ok {
		return nil, errors.New("sarama: async producer already instrumented")
	}
	if !cfg.Producer.Return.Successes || !cfg.Producer.Return.Errors {
		return nil, errors.New("sarama: async instrumentation requires Return.Successes and Return.Errors")
	}
	i, err := New(p, opts...)
	if err != nil {
		return nil, err
	}
	w := &AsyncProducer{AsyncProducer: producer, i: i, input: make(chan *native.ProducerMessage), requests: make(chan asyncRequest), transactions: make(chan func()), successes: make(chan *native.ProducerMessage), errors: make(chan *native.ProducerError), stop: make(chan struct{}), pending: make(map[*native.ProducerMessage]asyncCompletion)}
	go w.forward()
	go w.complete()
	return w, nil
}
func (w *AsyncProducer) Input() chan<- *native.ProducerMessage     { return w.input }
func (w *AsyncProducer) Successes() <-chan *native.ProducerMessage { return w.successes }
func (w *AsyncProducer) Errors() <-chan *native.ProducerError      { return w.errors }

// Send supplies a parent context for asynchronous enqueueing. A nil error means
// accepted by the wrapper, not delivered; delivery ends on Successes or Errors.
// Cancellation before acceptance does not submit the message.
func (w *AsyncProducer) Send(ctx context.Context, message *native.ProducerMessage) error {
	if message == nil {
		return errors.New("sarama: nil message")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-w.stop:
		return native.ErrShuttingDown
	default:
	}
	select {
	case w.requests <- asyncRequest{ctx, message}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return native.ErrShuttingDown
	}
}
func (w *AsyncProducer) forward() {
	defer w.AsyncProducer.AsyncClose()
	for {
		var req asyncRequest
		select {
		case <-w.stop:
			return
		case transaction := <-w.transactions:
			transaction()
			continue
		case msg, ok := <-w.input:
			if !ok {
				w.AsyncClose()
				return
			}
			req = asyncRequest{context.Background(), msg}
		case req = <-w.requests:
		}
		if req.message == nil {
			w.AsyncProducer.Input() <- nil
			continue
		}
		copy := *req.message
		copy.Headers = append([]native.RecordHeader(nil), req.message.Headers...)
		ctx, finish := w.i.t.Begin(req.ctx, "send", copy.Topic, trace.SpanKindProducer)
		w.i.t.Inject(ctx, HeaderCarrier{&copy.Headers})
		w.mu.Lock()
		w.pending[&copy] = asyncCompletion{req.message, finish}
		w.mu.Unlock()
		w.AsyncProducer.Input() <- &copy
	}
}

// Serialize transaction operations with the forwarding loop so a transaction
// cannot overtake a message already accepted through Input or Send.
func (w *AsyncProducer) transaction(operation func() error) error {
	done := make(chan error, 1)
	select {
	case w.transactions <- func() { done <- operation() }:
		return <-done
	case <-w.stop:
		return native.ErrShuttingDown
	}
}

func (w *AsyncProducer) BeginTxn() error  { return w.transaction(w.AsyncProducer.BeginTxn) }
func (w *AsyncProducer) CommitTxn() error { return w.transaction(w.AsyncProducer.CommitTxn) }
func (w *AsyncProducer) AbortTxn() error  { return w.transaction(w.AsyncProducer.AbortTxn) }
func (w *AsyncProducer) AddOffsetsToTxn(offsets map[string][]*native.PartitionOffsetMetadata, groupID string) error {
	return w.transaction(func() error { return w.AsyncProducer.AddOffsetsToTxn(offsets, groupID) })
}
func (w *AsyncProducer) AddOffsetsToTxnWithGroupMetadata(offsets map[string][]*native.PartitionOffsetMetadata, metadata *native.ConsumerGroupMetadata) error {
	return w.transaction(func() error { return w.AsyncProducer.AddOffsetsToTxnWithGroupMetadata(offsets, metadata) })
}
func (w *AsyncProducer) AddMessageToTxn(message *native.ConsumerMessage, groupID string, metadata *string) error {
	return w.transaction(func() error { return w.AsyncProducer.AddMessageToTxn(message, groupID, metadata) })
}
func (w *AsyncProducer) AddMessageToTxnWithGroupMetadata(message *native.ConsumerMessage, group *native.ConsumerGroupMetadata, metadata *string) error {
	return w.transaction(func() error { return w.AsyncProducer.AddMessageToTxnWithGroupMetadata(message, group, metadata) })
}
func (w *AsyncProducer) restore(message *native.ProducerMessage, err error) *native.ProducerMessage {
	w.mu.Lock()
	state, ok := w.pending[message]
	delete(w.pending, message)
	w.mu.Unlock()
	if !ok {
		return message
	}
	state.finish(err)
	headers := state.original.Headers
	*state.original = *message
	state.original.Headers = headers
	return state.original
}
func (w *AsyncProducer) complete() {
	defer close(w.successes)
	defer close(w.errors)
	successes, failures := w.AsyncProducer.Successes(), w.AsyncProducer.Errors()
	for successes != nil || failures != nil {
		select {
		case message, ok := <-successes:
			if !ok {
				successes = nil
				continue
			}
			w.successes <- w.restore(message, nil)
		case failure, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			if failure != nil {
				failure.Msg = w.restore(failure.Msg, failure.Err)
			}
			w.errors <- failure
		}
	}
	// Native producers with both return channels enabled normally complete every
	// record. If one closes unexpectedly, do not leave unbounded pending spans.
	w.mu.Lock()
	defer w.mu.Unlock()
	for message, state := range w.pending {
		state.finish(native.ErrShuttingDown)
		delete(w.pending, message)
	}
}
func (w *AsyncProducer) AsyncClose() { w.once.Do(func() { close(w.stop) }) }
func (w *AsyncProducer) Close() error {
	w.AsyncClose()
	successes, failures := w.successes, w.errors
	var collected native.ProducerErrors
	for successes != nil || failures != nil {
		select {
		case _, ok := <-successes:
			if !ok {
				successes = nil
			}
		case failure, ok := <-failures:
			if !ok {
				failures = nil
			} else if failure != nil {
				collected = append(collected, failure)
			}
		}
	}
	if len(collected) > 0 {
		return collected
	}
	return nil
}

var _ native.AsyncProducer = (*AsyncProducer)(nil)
