package franzgo

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/codes"
)

type fakePoller struct {
	fetches kgo.Fetches
	max     int
}

func (p *fakePoller) PollRecords(_ context.Context, max int) kgo.Fetches {
	p.max = max
	return p.fetches
}

type missingRecordProducer struct{ err error }

func (p missingRecordProducer) Produce(_ context.Context, _ *kgo.Record, fn func(*kgo.Record, error)) {
	fn(nil, p.err)
}

func TestPollBatchAndNativeErrorIdentity(t *testing.T) {
	p := testkit.New(t)
	i, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	failure := errors.New("SECRET")
	poller := &fakePoller{fetches: kgo.Fetches{{Topics: []kgo.FetchTopic{{Partitions: []kgo.FetchPartition{{Err: failure}}}}}}}
	if result := i.PollRecords(ctx, poller, 15); result.Err() != failure || poller.max != 15 {
		t.Fatal("native polling contract")
	}
	if i.PollRecords(ctx, nil, 1).Err() == nil {
		t.Fatal("nil poller")
	}
	calls := 0
	err = i.ProcessBatch(ctx, []*kgo.Record{{Topic: "orders"}, {Topic: "orders"}}, func(context.Context, *kgo.Record) error { calls++; return failure })
	if !errors.Is(err, failure) || calls != 2 {
		t.Fatal("batch did not process all records")
	}
	for _, record := range []*kgo.Record{nil, {}} {
		i.Produce(ctx, nil, record, func(actual *kgo.Record, err error) {
			if actual != record || err == nil {
				t.Fatal("invalid producer callback")
			}
		})
	}
	i.Produce(ctx, nil, nil, nil)
	record := &kgo.Record{Topic: "orders", Offset: 7, Headers: []kgo.RecordHeader{{Key: "app", Value: []byte("kept")}}}
	i.Produce(ctx, missingRecordProducer{failure}, record, func(actual *kgo.Record, err error) {
		if actual != record || actual.Offset != 7 || err != failure {
			t.Fatal("missing broker record changed caller state")
		}
	})
	if i.Process(ctx, nil, func(context.Context, *kgo.Record) error { return nil }) == nil || i.Process(ctx, record, nil) == nil {
		t.Fatal("invalid processing")
	}
	headers := []kgo.RecordHeader{{Key: "TraceParent", Value: []byte("old")}, {Key: "app", Value: []byte("kept")}}
	carrier := HeaderCarrier{&headers}
	carrier.Set("traceparent", "new")
	if len(carrier.Keys()) != 2 || carrier.Get("TRACEPARENT") != "new" || carrier.Get("missing") != "" {
		t.Fatal("carrier contract")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestNativeHookSignalsAndPanicLifecycle(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			i, err := NewNative(p, WithTracing(signals[0]), WithMetrics(signals[1]))
			if err != nil {
				t.Fatal(err)
			}
			tracer := i.Hooks()[0].(*kotel.Tracer)
			meter := i.Hooks()[1].(*kotel.Meter)
			record := &kgo.Record{Context: context.Background(), Topic: "orders"}
			tracer.OnProduceRecordBuffered(record)
			tracer.OnProduceRecordUnbuffered(record, nil)
			meter.OnBrokerRead(kgo.BrokerMetadata{NodeID: 1}, 0, 64, 0, 0, nil)
			if err = i.Process(context.Background(), &kgo.Record{Topic: "orders"}, func(context.Context, *kgo.Record) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if (len(p.Recorder.Ended()) > 0) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("independent signals")
			}
		})
	}
	p := testkit.New(t)
	i, err := NewNative(p)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("SECRET")
	func() {
		defer func() {
			if recover() != failure {
				t.Fatal("panic identity")
			}
		}()
		_ = i.Process(context.Background(), &kgo.Record{Topic: "orders"}, func(context.Context, *kgo.Record) error { panic(failure) })
	}()
	spans := p.Recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error || len(spans[0].Events()) != 1 {
		t.Fatal("panic lifetime")
	}
	if i.Process(context.Background(), nil, func(context.Context, *kgo.Record) error { return nil }) == nil || i.Process(context.Background(), &kgo.Record{}, nil) == nil {
		t.Fatal("invalid native process")
	}
	p.AssertPrivate(t, "SECRET")
}
