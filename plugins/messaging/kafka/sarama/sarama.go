// Package sarama provides compatibility instrumentation for modern IBM Sarama.
package sarama

import (
	"context"
	"errors"
	"strings"

	native "github.com/IBM/sarama"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/messaging/kafka/sarama"

type Option = telemetry.Option

var WithDestinationNormalizer = telemetry.WithDestinationNormalizer
var WithConsumerGroup = telemetry.WithConsumerGroup
var WithClientID = telemetry.WithClientID
var WithTraceAttributes = telemetry.WithTraceAttributes
var WithMetricAttributes = telemetry.WithMetricAttributes
var WithPropagator = telemetry.WithPropagator
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type HeaderCarrier struct{ Headers *[]native.RecordHeader }

func (h HeaderCarrier) Get(key string) string {
	for _, v := range *h.Headers {
		if strings.EqualFold(string(v.Key), key) {
			return string(v.Value)
		}
	}
	return ""
}
func (h HeaderCarrier) Set(key, value string) {
	out := make([]native.RecordHeader, 0, len(*h.Headers)+1)
	for _, v := range *h.Headers {
		if !strings.EqualFold(string(v.Key), key) {
			out = append(out, v)
		}
	}
	*h.Headers = append(out, native.RecordHeader{Key: []byte(key), Value: []byte(value)})
}
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*h.Headers))
	for _, v := range *h.Headers {
		keys = append(keys, string(v.Key))
	}
	return keys
}

type SyncProducer interface {
	SendMessage(*native.ProducerMessage) (int32, int64, error)
	SendMessages([]*native.ProducerMessage) error
}
type Instrumentation struct{ t *telemetry.Instrumentation }

func New(p goslogx.TelemetryProvider, opts ...Option) (*Instrumentation, error) {
	t, err := telemetry.New(p, ScopeName, "kafka", true, opts...)
	if err != nil {
		return nil, err
	}
	return &Instrumentation{t}, nil
}
func (i *Instrumentation) SendMessage(ctx context.Context, producer SyncProducer, message *native.ProducerMessage) (partition int32, offset int64, err error) {
	if telemetry.Nil(producer) || message == nil {
		return 0, 0, errors.New("sarama instrumentation: nil producer or message")
	}
	ctx, finish := i.t.Begin(ctx, "send", message.Topic, trace.SpanKindProducer)
	defer func() { finish(err) }()
	copy := *message
	copy.Headers = append([]native.RecordHeader(nil), message.Headers...)
	i.t.Inject(ctx, HeaderCarrier{&copy.Headers})
	partition, offset, err = producer.SendMessage(&copy)
	headers := message.Headers
	*message = copy
	message.Headers = headers
	return partition, offset, err
}

// SendMessages keeps the native batch call, error identity, retries and partition assignments.
func (i *Instrumentation) SendMessages(ctx context.Context, producer SyncProducer, messages []*native.ProducerMessage) (err error) {
	if telemetry.Nil(producer) {
		return errors.New("sarama instrumentation: nil producer")
	}
	copies := make([]*native.ProducerMessage, len(messages))
	finishes := make([]func(error), len(messages))
	for index, msg := range messages {
		if msg == nil {
			for _, finish := range finishes {
				if finish != nil {
					finish(errors.New("nil message"))
				}
			}
			return errors.New("sarama instrumentation: nil message")
		}
		copy := *msg
		copy.Headers = append([]native.RecordHeader(nil), msg.Headers...)
		child, finish := i.t.Begin(ctx, "send", msg.Topic, trace.SpanKindProducer)
		finishes[index] = finish
		i.t.Inject(child, HeaderCarrier{&copy.Headers})
		copies[index] = &copy
	}
	err = producer.SendMessages(copies)
	perMessage := make(map[*native.ProducerMessage]error)
	originals := make(map[*native.ProducerMessage]*native.ProducerMessage, len(copies))
	for index, copy := range copies {
		originals[copy] = messages[index]
	}
	var batchErrors native.ProducerErrors
	if errors.As(err, &batchErrors) {
		for _, failure := range batchErrors {
			perMessage[failure.Msg] = failure.Err
		}
	}
	for index, copy := range copies {
		recordErr := err
		if batchErrors != nil {
			recordErr = perMessage[copy]
		}
		finishes[index](recordErr)
		headers := messages[index].Headers
		*messages[index] = *copy
		messages[index].Headers = headers
	}
	for _, failure := range batchErrors {
		if original, ok := originals[failure.Msg]; ok {
			failure.Msg = original
		}
	}
	return err
}
func (i *Instrumentation) Process(ctx context.Context, message *native.ConsumerMessage, handler func(context.Context, *native.ConsumerMessage) error) error {
	if message == nil || handler == nil {
		return errors.New("sarama instrumentation: nil message or handler")
	}
	headers := make([]native.RecordHeader, 0, len(message.Headers))
	for _, h := range message.Headers {
		if h != nil {
			headers = append(headers, *h)
		}
	}
	return i.t.Process(ctx, HeaderCarrier{&headers}, message.Topic, func(ctx context.Context) error { return handler(ctx, message) })
}

// GroupHandler preserves Setup/Cleanup, claims and native message commit decisions.
// Handle receives the process context explicitly; the native session context cannot vary per message.
func (i *Instrumentation) GroupHandler(setup func(native.ConsumerGroupSession) error, cleanup func(native.ConsumerGroupSession) error, handle func(context.Context, native.ConsumerGroupSession, *native.ConsumerMessage) error) native.ConsumerGroupHandler {
	return &groupHandler{i, setup, cleanup, handle}
}

type groupHandler struct {
	i              *Instrumentation
	setup, cleanup func(native.ConsumerGroupSession) error
	handle         func(context.Context, native.ConsumerGroupSession, *native.ConsumerMessage) error
}

func (h *groupHandler) Setup(s native.ConsumerGroupSession) error {
	if h.setup != nil {
		return h.setup(s)
	}
	return nil
}
func (h *groupHandler) Cleanup(s native.ConsumerGroupSession) error {
	if h.cleanup != nil {
		return h.cleanup(s)
	}
	return nil
}
func (h *groupHandler) ConsumeClaim(s native.ConsumerGroupSession, claim native.ConsumerGroupClaim) error {
	if h.handle == nil {
		return errors.New("sarama instrumentation: nil group handler")
	}
	for {
		select {
		case <-s.Context().Done():
			return s.Context().Err()
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			if err := h.i.Process(s.Context(), msg, func(ctx context.Context, msg *native.ConsumerMessage) error { return h.handle(ctx, s, msg) }); err != nil {
				return err
			}
		}
	}
}
