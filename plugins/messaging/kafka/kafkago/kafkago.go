// Package kafkago instruments segmentio/kafka-go through native reader/writer interfaces.
package kafkago

import (
	"context"
	"errors"
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	kafka "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/messaging/kafka/kafkago"

type Option = telemetry.Option

var WithDestinationNormalizer = telemetry.WithDestinationNormalizer
var WithConsumerGroup = telemetry.WithConsumerGroup
var WithClientID = telemetry.WithClientID
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type HeaderCarrier struct{ Headers *[]kafka.Header }

func (h HeaderCarrier) Get(key string) string {
	for _, v := range *h.Headers {
		if strings.EqualFold(v.Key, key) {
			return string(v.Value)
		}
	}
	return ""
}
func (h HeaderCarrier) Set(key, value string) {
	out := make([]kafka.Header, 0, len(*h.Headers)+1)
	for _, v := range *h.Headers {
		if !strings.EqualFold(v.Key, key) {
			out = append(out, v)
		}
	}
	*h.Headers = append(out, kafka.Header{Key: key, Value: []byte(value)})
}
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*h.Headers))
	for _, v := range *h.Headers {
		keys = append(keys, v.Key)
	}
	return keys
}

type Writer interface {
	WriteMessages(context.Context, ...kafka.Message) error
}
type Reader interface {
	FetchMessage(context.Context) (kafka.Message, error)
}
type Instrumentation struct{ t *telemetry.Instrumentation }

func New(p goslogx.TelemetryProvider, opts ...Option) (*Instrumentation, error) {
	t, err := telemetry.New(p, ScopeName, "kafka", true, opts...)
	if err != nil {
		return nil, err
	}
	return &Instrumentation{t}, nil
}

// WriteMessages preserves batch writes and per-message errors. Use a synchronous Writer
// (Async=false); asynchronous writers acknowledge enqueueing rather than delivery.
func (i *Instrumentation) WriteMessages(ctx context.Context, writer Writer, messages ...kafka.Message) (err error) {
	if telemetry.Nil(writer) {
		return errors.New("kafkago instrumentation: nil writer")
	}
	if native, ok := writer.(*kafka.Writer); ok && native.Async {
		return errors.New("kafkago instrumentation: async writer requires native completion-based instrumentation")
	}
	copies := append([]kafka.Message(nil), messages...)
	finishes := make([]func(error), len(copies))
	for index := range copies {
		destination := copies[index].Topic
		if native, ok := writer.(*kafka.Writer); ok && destination == "" {
			destination = native.Topic
		}
		child, finish := i.t.Begin(ctx, "send", destination, trace.SpanKindProducer)
		finishes[index] = finish
		copies[index].Headers = append([]kafka.Header(nil), copies[index].Headers...)
		i.t.Inject(child, HeaderCarrier{&copies[index].Headers})
	}
	defer func() {
		for index, finish := range finishes {
			recordErr := err
			var writeErrors kafka.WriteErrors
			if errors.As(err, &writeErrors) && index < len(writeErrors) {
				recordErr = writeErrors[index]
			}
			finish(recordErr)
		}
	}()
	return writer.WriteMessages(ctx, copies...)
}
func (i *Instrumentation) FetchMessage(ctx context.Context, reader Reader) (message kafka.Message, err error) {
	if telemetry.Nil(reader) {
		return message, errors.New("kafkago instrumentation: nil reader")
	}
	_, finish := i.t.BeginCount(ctx, "receive", "", trace.SpanKindConsumer, 0)
	defer func() { finish(err) }()
	return reader.FetchMessage(ctx)
}

// Process does not commit; retain your native consumer-group commit policy.
func (i *Instrumentation) Process(ctx context.Context, message kafka.Message, handler func(context.Context, kafka.Message) error) error {
	if handler == nil {
		return errors.New("kafkago instrumentation: nil handler")
	}
	return i.t.Process(ctx, HeaderCarrier{&message.Headers}, message.Topic, func(ctx context.Context) error { return handler(ctx, message) })
}
func (i *Instrumentation) ProcessBatch(ctx context.Context, messages []kafka.Message, handler func(context.Context, kafka.Message) error) error {
	var errs []error
	for _, msg := range messages {
		if err := i.Process(ctx, msg, handler); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
