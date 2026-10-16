// Package franzgo provides explicit helpers around franz-go's native producer and fetch APIs.
package franzgo

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/messaging/kafka/franzgo"

type Option = telemetry.Option

var WithDestinationNormalizer = telemetry.WithDestinationNormalizer
var WithConsumerGroup = telemetry.WithConsumerGroup
var WithClientID = telemetry.WithClientID
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type HeaderCarrier struct{ Headers *[]kgo.RecordHeader }

func (h HeaderCarrier) Get(key string) string {
	for _, v := range *h.Headers {
		if strings.EqualFold(v.Key, key) {
			return string(v.Value)
		}
	}
	return ""
}
func (h HeaderCarrier) Set(key, value string) {
	out := make([]kgo.RecordHeader, 0, len(*h.Headers)+1)
	for _, v := range *h.Headers {
		if !strings.EqualFold(v.Key, key) {
			out = append(out, v)
		}
	}
	*h.Headers = append(out, kgo.RecordHeader{Key: key, Value: []byte(value)})
}
func (h HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(*h.Headers))
	for _, v := range *h.Headers {
		keys = append(keys, v.Key)
	}
	return keys
}

type Producer interface {
	Produce(context.Context, *kgo.Record, func(*kgo.Record, error))
}
type Poller interface {
	PollRecords(context.Context, int) kgo.Fetches
}
type Instrumentation struct{ t *telemetry.Instrumentation }

func New(p goslogx.TelemetryProvider, opts ...Option) (*Instrumentation, error) {
	t, err := telemetry.New(p, ScopeName, "kafka", true, opts...)
	if err != nil {
		return nil, err
	}
	return &Instrumentation{t}, nil
}

// Produce retains native retries/cancellation and reports the native acknowledgement.
// Caller headers are not mutated; broker-assigned metadata is copied back on completion.
func (i *Instrumentation) Produce(ctx context.Context, producer Producer, record *kgo.Record, promise func(*kgo.Record, error)) {
	if telemetry.Nil(producer) || record == nil {
		if promise != nil {
			promise(record, errors.New("franzgo instrumentation: nil producer or record"))
		}
		return
	}
	originalCtx := record.Context
	child, finish := i.t.Begin(ctx, "send", record.Topic, trace.SpanKindProducer)
	copy := *record
	copy.Headers = append([]kgo.RecordHeader(nil), record.Headers...)
	copy.Context = child
	i.t.Inject(child, HeaderCarrier{&copy.Headers})
	producer.Produce(child, &copy, func(produced *kgo.Record, err error) {
		if produced != nil {
			headers := record.Headers
			*record = *produced
			record.Headers = headers
			record.Context = originalCtx
		}
		finish(err)
		if promise != nil {
			promise(record, err)
		}
	})
}

// ProduceSync delegates each record to native Produce and waits for its callback.
func (i *Instrumentation) ProduceSync(ctx context.Context, producer Producer, records ...*kgo.Record) kgo.ProduceResults {
	results := make(kgo.ProduceResults, len(records))
	var wg sync.WaitGroup
	for index, record := range records {
		wg.Add(1)
		i.Produce(ctx, producer, record, func(record *kgo.Record, err error) {
			results[index] = kgo.ProduceResult{Record: record, Err: err}
			wg.Done()
		})
	}
	wg.Wait()
	return results
}
func (i *Instrumentation) PollRecords(ctx context.Context, client Poller, maxRecords int) kgo.Fetches {
	// The native client controls fetch/group/commit/rebalance semantics.
	_, finish := i.t.BeginCount(ctx, "receive", "", trace.SpanKindConsumer, 0)
	result := client.PollRecords(ctx, maxRecords)
	finish(result.Err())
	return result
}
func (i *Instrumentation) Process(ctx context.Context, record *kgo.Record, handler func(context.Context, *kgo.Record) error) error {
	if record == nil || handler == nil {
		return errors.New("franzgo instrumentation: nil record or handler")
	}
	return i.t.Process(ctx, HeaderCarrier{&record.Headers}, record.Topic, func(ctx context.Context) error { return handler(ctx, record) })
}
func (i *Instrumentation) ProcessBatch(ctx context.Context, records []*kgo.Record, handler func(context.Context, *kgo.Record) error) error {
	var errs []error
	for _, record := range records {
		if err := i.Process(ctx, record, handler); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
