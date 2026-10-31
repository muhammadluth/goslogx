package kafkago

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"weak"

	"github.com/muhammadluth/goslogx"
	kafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"
)

var asyncWriters sync.Map

// AsyncWriter preserves native delivery and Close behavior. Configure it before
// first use; completion callbacks, not enqueueing, end its producer spans.
type AsyncWriter struct {
	*kafka.Writer
	i    *Instrumentation
	send Writer
}
type completion struct {
	finish func(error)
	data   any
}

// WrapAsyncWriter requires an already asynchronous writer and never changes its
// delivery mode. Existing Completion callbacks receive their original WriterData.
func WrapAsyncWriter(writer *kafka.Writer, p goslogx.TelemetryProvider, opts ...Option) (*AsyncWriter, error) {
	if writer == nil || !writer.Async {
		return nil, errors.New("kafkago: WrapAsyncWriter requires Async=true")
	}
	i, err := New(p, opts...)
	if err != nil {
		return nil, err
	}
	key := weak.Make(writer)
	if _, loaded := asyncWriters.LoadOrStore(key, struct{}{}); loaded {
		return nil, errors.New("kafkago: writer already instrumented")
	}
	runtime.AddCleanup(writer, func(key weak.Pointer[kafka.Writer]) { asyncWriters.Delete(key) }, key)
	previous := writer.Completion
	writer.Completion = func(messages []kafka.Message, err error) {
		restored := append([]kafka.Message(nil), messages...)
		for n, msg := range restored {
			if state, ok := msg.WriterData.(*completion); ok {
				state.finish(err)
				restored[n].WriterData = state.data
			}
		}
		if previous != nil {
			previous(restored, err)
		}
	}
	return &AsyncWriter{Writer: writer, i: i, send: writer}, nil
}

func (w *AsyncWriter) WriteMessages(ctx context.Context, messages ...kafka.Message) error {
	copies := append([]kafka.Message(nil), messages...)
	for n := range copies {
		topic := copies[n].Topic
		if topic == "" {
			topic = w.Writer.Topic
		}
		child, finish := w.i.t.Begin(ctx, "send", topic, trace.SpanKindProducer)
		copies[n].WriterData = &completion{finish: finish, data: copies[n].WriterData}
		copies[n].Headers = append([]kafka.Header(nil), copies[n].Headers...)
		w.i.t.Inject(child, HeaderCarrier{&copies[n].Headers})
	}
	err := w.send.WriteMessages(ctx, copies...)
	// With Async=true, non-nil errors occur before batch enqueueing in v0.4.51.
	// Accepted records are completed solely by the native callback.
	if err != nil {
		for _, msg := range copies {
			msg.WriterData.(*completion).finish(err)
		}
	}
	return err
}
