package franzgo

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

type fakeProducer struct {
	records []*kgo.Record
	err     error
}

func (p *fakeProducer) Produce(_ context.Context, r *kgo.Record, callback func(*kgo.Record, error)) {
	p.records = append(p.records, r)
	r.Offset = 42
	callback(r, p.err)
}
func TestBatchAndProcess(t *testing.T) {
	p := testkit.New(t)
	instrument, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	producer := &fakeProducer{}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	record := &kgo.Record{Topic: "topic.SECRET", Key: []byte("SECRET"), Value: []byte("SECRET")}
	results := instrument.ProduceSync(ctx, producer, record)
	if results.FirstErr() != nil || record.Offset != 42 || len(record.Headers) != 0 {
		t.Fatal("producer semantics changed")
	}
	if err := instrument.Process(context.Background(), producer.records[0], func(ctx context.Context, r *kgo.Record) error {
		if trace.SpanContextFromContext(ctx).TraceID() != parent.SpanContext().TraceID() {
			t.Fatal("trace continuity")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	producer.err = errors.New("SECRET error")
	if instrument.ProduceSync(ctx, producer, &kgo.Record{Topic: "topic.SECRET"}).FirstErr() != producer.err {
		t.Fatal("error changed")
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}
