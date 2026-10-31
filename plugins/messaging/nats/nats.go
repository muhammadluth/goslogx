// Package nats instruments Core NATS and JetStream with explicit providers.
package nats

import (
	"context"
	"errors"
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/messaging/nats"

type Option = telemetry.Option

var WithDestinationNormalizer = telemetry.WithDestinationNormalizer
var WithConsumerName = telemetry.WithConsumerName
var WithTraceAttributes = telemetry.WithTraceAttributes
var WithMetricAttributes = telemetry.WithMetricAttributes
var WithPropagator = telemetry.WithPropagator
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics
var WithErrorHandler = telemetry.WithErrorHandler

// HeaderCarrier adapts native headers for the configured OpenTelemetry propagator.
type HeaderCarrier gonats.Header

func (h HeaderCarrier) Get(key string) string {
	if values := h[key]; len(values) > 0 {
		return values[0]
	}
	for existing, values := range h {
		if strings.EqualFold(existing, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
func (h HeaderCarrier) Set(key, value string) {
	for existing := range h {
		if strings.EqualFold(existing, key) {
			delete(h, existing)
		}
	}
	gonats.Header(h).Set(key, value)
}
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}
func clone(msg *gonats.Msg) *gonats.Msg {
	copy := *msg
	copy.Header = make(gonats.Header, len(msg.Header))
	for k, v := range msg.Header {
		copy.Header[k] = append([]string(nil), v...)
	}
	return &copy
}

type Client struct {
	*gonats.Conn
	t *telemetry.Instrumentation
}

func Wrap(conn *gonats.Conn, p goslogx.TelemetryProvider, opts ...Option) (*Client, error) {
	if conn == nil {
		return nil, errors.New("nats instrumentation: nil connection")
	}
	t, err := telemetry.New(p, ScopeName, "nats", true, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn, t}, nil
}
func (c *Client) Publish(ctx context.Context, subject string, data []byte) error {
	return c.PublishMsg(ctx, &gonats.Msg{Subject: subject, Data: data})
}
func (c *Client) PublishMsg(ctx context.Context, msg *gonats.Msg) (err error) {
	if msg == nil {
		return errors.New("nats instrumentation: nil message")
	}
	ctx, finish := c.t.Begin(ctx, "publish", msg.Subject, trace.SpanKindProducer)
	defer func() { finish(err) }()
	if err = ctx.Err(); err != nil {
		return err
	}
	copy := clone(msg)
	if c.Conn.HeadersSupported() {
		c.t.Inject(ctx, HeaderCarrier(copy.Header))
	}
	return c.Conn.PublishMsg(copy)
}
func (c *Client) Request(ctx context.Context, subject string, data []byte) (*gonats.Msg, error) {
	return c.RequestMsg(ctx, &gonats.Msg{Subject: subject, Data: data})
}
func (c *Client) RequestMsg(ctx context.Context, msg *gonats.Msg) (response *gonats.Msg, err error) {
	if msg == nil {
		return nil, errors.New("nats instrumentation: nil message")
	}
	ctx, finish := c.t.Begin(ctx, "request", msg.Subject, trace.SpanKindProducer)
	defer func() { finish(err) }()
	copy := clone(msg)
	if c.Conn.HeadersSupported() {
		c.t.Inject(ctx, HeaderCarrier(copy.Header))
	}
	return c.Conn.RequestMsgWithContext(ctx, copy)
}

// Subscribe does not auto-ack or alter handler errors. Use WithErrorHandler for asynchronous errors.
func (c *Client) Subscribe(ctx context.Context, subject string, handler func(context.Context, *gonats.Msg) error) (*gonats.Subscription, error) {
	if handler == nil {
		return nil, errors.New("nats instrumentation: nil handler")
	}
	return c.Conn.Subscribe(subject, func(msg *gonats.Msg) { c.t.Report(c.Process(ctx, msg, handler)) })
}
func (c *Client) QueueSubscribe(ctx context.Context, subject, queue string, handler func(context.Context, *gonats.Msg) error) (*gonats.Subscription, error) {
	if handler == nil {
		return nil, errors.New("nats instrumentation: nil handler")
	}
	return c.Conn.QueueSubscribe(subject, queue, func(msg *gonats.Msg) { c.t.Report(c.Process(ctx, msg, handler)) })
}
func (c *Client) Process(ctx context.Context, msg *gonats.Msg, handler func(context.Context, *gonats.Msg) error) error {
	if msg == nil || handler == nil {
		return errors.New("nats instrumentation: nil message or handler")
	}
	return c.t.Process(ctx, HeaderCarrier(msg.Header), msg.Subject, func(ctx context.Context) error { return handler(ctx, msg) })
}

type JetStream struct {
	jetstream.JetStream
	t                        *telemetry.Instrumentation
	redeliveries             metric.Int64Counter
	pending, ackPending, lag metric.Int64Gauge
}

func WrapJetStream(js jetstream.JetStream, p goslogx.TelemetryProvider, opts ...Option) (*JetStream, error) {
	if telemetry.Nil(js) {
		return nil, errors.New("nats instrumentation: nil JetStream")
	}
	t, err := telemetry.New(p, ScopeName, "nats", true, opts...)
	if err != nil {
		return nil, err
	}
	wrapped := &JetStream{JetStream: js, t: t}
	wrapped.redeliveries, err = t.Meter.Int64Counter("messaging.nats.redelivered.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	wrapped.pending, err = t.Meter.Int64Gauge("messaging.nats.consumer.pending.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	wrapped.ackPending, err = t.Meter.Int64Gauge("messaging.nats.consumer.ack_pending.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	wrapped.lag, err = t.Meter.Int64Gauge("messaging.nats.consumer.lag.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	return wrapped, nil
}
func (j *JetStream) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	return j.PublishMsg(ctx, &gonats.Msg{Subject: subject, Data: data}, opts...)
}
func (j *JetStream) PublishMsg(ctx context.Context, msg *gonats.Msg, opts ...jetstream.PublishOpt) (ack *jetstream.PubAck, err error) {
	if msg == nil {
		return nil, errors.New("nats instrumentation: nil message")
	}
	ctx, finish := j.t.Begin(ctx, "publish", msg.Subject, trace.SpanKindProducer)
	defer func() { finish(err) }()
	copy := clone(msg)
	j.t.Inject(ctx, HeaderCarrier(copy.Header))
	return j.JetStream.PublishMsg(ctx, copy, opts...)
}
func (j *JetStream) Process(ctx context.Context, msg jetstream.Msg, handler func(context.Context, jetstream.Msg) error) error {
	if telemetry.Nil(msg) || handler == nil {
		return errors.New("nats instrumentation: nil message or handler")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if metadata, err := msg.Metadata(); err == nil && metadata.NumDelivered > 1 {
		j.redeliveries.Add(ctx, 1, metric.WithAttributes(append([]attribute.KeyValue{attribute.String("messaging.system", "nats")}, j.t.Config.MetricAttributes...)...))
	}
	return j.t.Process(ctx, HeaderCarrier(msg.Headers()), msg.Subject(), func(ctx context.Context) error { return handler(ctx, msg) })
}
func (j *JetStream) Consume(ctx context.Context, consumer jetstream.Consumer, handler func(context.Context, jetstream.Msg) error, opts ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	if telemetry.Nil(consumer) || handler == nil {
		return nil, errors.New("nats instrumentation: nil consumer or handler")
	}
	return consumer.Consume(func(msg jetstream.Msg) { j.t.Report(j.Process(ctx, msg, handler)) }, opts...)
}

// Fetch measures native batch completion, including asynchronous delivery and
// errors. Call Process for each returned message to measure application work.
func (j *JetStream) Fetch(ctx context.Context, consumer jetstream.Consumer, batch int, opts ...jetstream.FetchOpt) (messages jetstream.MessageBatch, err error) {
	if telemetry.Nil(consumer) {
		return nil, errors.New("nats instrumentation: nil consumer")
	}
	_, finish := j.t.BeginCount(ctx, "fetch", "", trace.SpanKindConsumer, 0)
	if ctx != nil && ctx.Err() != nil {
		finish(ctx.Err())
		return nil, ctx.Err()
	}
	messages, err = consumer.Fetch(batch, opts...)
	if err != nil {
		finish(err)
		return messages, err
	}
	nativeBatch := messages
	in := nativeBatch.Messages()
	out := make(chan jetstream.Msg, cap(in))
	observed := &observedBatch{MessageBatch: messages, messages: out}
	go func() {
		for msg := range in {
			out <- msg
		}
		finish(nativeBatch.Error())
		close(out)
	}()
	return observed, nil
}

// observedBatch retains native error access and a stable receive channel. Its
// buffer matches the native batch, so delivery does not require caller draining.
type observedBatch struct {
	jetstream.MessageBatch
	messages <-chan jetstream.Msg
}

func (b *observedBatch) Messages() <-chan jetstream.Msg { return b.messages }

// ObserveConsumer records a caller-refreshed snapshot without broker API calls or polling goroutines.
// Lag means outstanding work: pending delivery plus pending acknowledgement.
func (j *JetStream) ObserveConsumer(ctx context.Context, info *jetstream.ConsumerInfo) {
	if ctx == nil {
		ctx = context.Background()
	}
	if info == nil {
		return
	}
	attrs := append([]attribute.KeyValue{attribute.String("messaging.system", "nats")}, j.t.Config.MetricAttributes...)
	if j.t.Config.Consumer != "" {
		attrs = append(attrs, attribute.String("messaging.consumer.name", j.t.Config.Consumer))
	}
	options := metric.WithAttributes(attrs...)
	j.pending.Record(ctx, int64(info.NumPending), options)
	j.ackPending.Record(ctx, int64(info.NumAckPending), options)
	j.lag.Record(ctx, int64(info.NumPending)+int64(info.NumAckPending), options)
}

var _ jetstream.JetStream = (*JetStream)(nil)
