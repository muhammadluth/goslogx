// Package rabbitmq instruments amqp091-go without changing delivery acknowledgement policy.
package rabbitmq

import (
	"context"
	"errors"
	"strings"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/messaging/rabbitmq"

type Option = telemetry.Option

var WithDestinationNormalizer = telemetry.WithDestinationNormalizer
var WithConsumerName = telemetry.WithConsumerName

func WithRoutingKeyNormalizer(f func(string) string) Option {
	return func(c *telemetry.Config) { c.RoutingNormalizer = f }
}

var WithTraceAttributes = telemetry.WithTraceAttributes
var WithMetricAttributes = telemetry.WithMetricAttributes
var WithPropagator = telemetry.WithPropagator
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type HeaderCarrier amqp.Table

func (h HeaderCarrier) Get(key string) string {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			switch value := v.(type) {
			case string:
				return value
			case []byte:
				return string(value)
			}
		}
	}
	return ""
}
func (h HeaderCarrier) Set(key, value string) {
	for k := range h {
		if strings.EqualFold(k, key) {
			delete(h, k)
		}
	}
	h[key] = value
}
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

type Channel interface {
	PublishWithContext(context.Context, string, string, bool, bool, amqp.Publishing) error
	ConsumeWithContext(context.Context, string, string, bool, bool, bool, bool, amqp.Table) (<-chan amqp.Delivery, error)
}
type Client struct {
	Channel                   Channel
	t                         *telemetry.Instrumentation
	settlements, redeliveries metric.Int64Counter
}

func WrapChannel(channel Channel, p goslogx.TelemetryProvider, opts ...Option) (*Client, error) {
	if telemetry.Nil(channel) {
		return nil, errors.New("rabbitmq instrumentation: nil channel")
	}
	t, err := telemetry.New(p, ScopeName, "rabbitmq", true, opts...)
	if err != nil {
		return nil, err
	}
	settled, err := t.Meter.Int64Counter("messaging.rabbitmq.settlements", metric.WithUnit("{operation}"))
	if err != nil {
		return nil, err
	}
	redelivered, err := t.Meter.Int64Counter("messaging.rabbitmq.redelivered.messages", metric.WithUnit("{message}"))
	if err != nil {
		return nil, err
	}
	return &Client{channel, t, settled, redelivered}, nil
}

// Publish retains exchange, routing key, reply/correlation fields, flags and payload.
// Only explicitly normalized exchange names become telemetry dimensions.
func (c *Client) Publish(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) (err error) {
	ctx, finish := c.t.Begin(ctx, "publish", exchange, trace.SpanKindProducer)
	defer func() { finish(err) }()
	if normalize := c.t.Config.RoutingNormalizer; normalize != nil {
		normalized := normalize(key)
		if len(normalized) > 0 && len(normalized) <= 128 {
			trace.SpanFromContext(ctx).SetAttributes(attribute.String("messaging.rabbitmq.destination.routing_key", normalized))
		}
	}
	headers := make(amqp.Table, len(msg.Headers))
	for k, v := range msg.Headers {
		headers[k] = v
	}
	msg.Headers = headers
	c.t.Inject(ctx, HeaderCarrier(headers))
	return c.Channel.PublishWithContext(ctx, exchange, key, mandatory, immediate, msg)
}

// Consume returns the native delivery stream. Process each delivery explicitly.
func (c *Client) Consume(ctx context.Context, queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (deliveries <-chan amqp.Delivery, err error) {
	ctx, finish := c.t.BeginCount(ctx, "consume", queue, trace.SpanKindConsumer, 0)
	defer func() { finish(err) }()
	return c.Channel.ConsumeWithContext(ctx, queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}
func (c *Client) Process(ctx context.Context, queue string, delivery amqp.Delivery, handler func(context.Context, amqp.Delivery) error) error {
	if handler == nil {
		return errors.New("rabbitmq instrumentation: nil handler")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if delivery.Redelivered {
		c.redeliveries.Add(ctx, 1, metric.WithAttributes(append([]attribute.KeyValue{attribute.String("messaging.system", "rabbitmq")}, c.t.Config.MetricAttributes...)...))
	}
	return c.t.Process(ctx, HeaderCarrier(delivery.Headers), queue, func(ctx context.Context) error {
		if !telemetry.Nil(delivery.Acknowledger) {
			delivery.Acknowledger = acknowledger{delivery.Acknowledger, c, ctx, queue}
		}
		return handler(ctx, delivery)
	})
}

type acknowledger struct {
	delegate amqp.Acknowledger
	client   *Client
	ctx      context.Context
	queue    string
}

func (a acknowledger) settle(op string, fn func() error) (err error) {
	ctx, finish := a.client.t.Begin(a.ctx, op, a.queue, trace.SpanKindConsumer)
	defer func() { finish(err) }()
	err = fn()
	if err == nil {
		a.client.settlements.Add(ctx, 1, metric.WithAttributes(append([]attribute.KeyValue{attribute.String("messaging.system", "rabbitmq"), attribute.String("messaging.operation.name", op)}, a.client.t.Config.MetricAttributes...)...))
	}
	return err
}
func (a acknowledger) Ack(tag uint64, multiple bool) error {
	return a.settle("ack", func() error { return a.delegate.Ack(tag, multiple) })
}
func (a acknowledger) Nack(tag uint64, multiple, requeue bool) error {
	return a.settle("nack", func() error { return a.delegate.Nack(tag, multiple, requeue) })
}
func (a acknowledger) Reject(tag uint64, requeue bool) error {
	return a.settle("reject", func() error { return a.delegate.Reject(tag, requeue) })
}

// Ack/Nack/Reject also instrument deliveries processed outside a callback.
func (c *Client) Ack(ctx context.Context, queue string, delivery amqp.Delivery, multiple bool) error {
	if telemetry.Nil(delivery.Acknowledger) {
		return errors.New("rabbitmq instrumentation: no acknowledger")
	}
	if current, ok := delivery.Acknowledger.(acknowledger); ok && current.client == c {
		return current.Ack(delivery.DeliveryTag, multiple)
	}
	return (acknowledger{delivery.Acknowledger, c, ctx, queue}).Ack(delivery.DeliveryTag, multiple)
}
func (c *Client) Nack(ctx context.Context, queue string, delivery amqp.Delivery, multiple, requeue bool) error {
	if telemetry.Nil(delivery.Acknowledger) {
		return errors.New("rabbitmq instrumentation: no acknowledger")
	}
	if current, ok := delivery.Acknowledger.(acknowledger); ok && current.client == c {
		return current.Nack(delivery.DeliveryTag, multiple, requeue)
	}
	return (acknowledger{delivery.Acknowledger, c, ctx, queue}).Nack(delivery.DeliveryTag, multiple, requeue)
}
func (c *Client) Reject(ctx context.Context, queue string, delivery amqp.Delivery, requeue bool) error {
	if telemetry.Nil(delivery.Acknowledger) {
		return errors.New("rabbitmq instrumentation: no acknowledger")
	}
	if current, ok := delivery.Acknowledger.(acknowledger); ok && current.client == c {
		return current.Reject(delivery.DeliveryTag, requeue)
	}
	return (acknowledger{delivery.Acknowledger, c, ctx, queue}).Reject(delivery.DeliveryTag, requeue)
}
