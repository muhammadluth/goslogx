package franzgo

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"strings"
	"testing"
)

func TestNativeHooksCompletionPropagationAndMetrics(t *testing.T) {
	p := testkit.New(t)
	i, err := NewNative(p, WithConsumerLinks(true), WithConsumerGroup("workers"), WithClientID("producer"), WithDestinationNormalizer(func(string) string { return "orders" }), WithMetricAttributes(attribute.String("app.region", "local")))
	if err != nil {
		t.Fatal(err)
	}
	hooks := i.Hooks()
	tracer := hooks[0].(*kotel.Tracer)
	meter := hooks[1].(*kotel.Meter)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "request")
	record := &kgo.Record{Context: ctx, Topic: "orders.tenant", Key: []byte("SECRET"), Value: []byte("SECRET")}
	tracer.OnProduceRecordBuffered(record)
	if len(p.Recorder.Ended()) != 0 {
		t.Fatal("producer ended before acknowledgement")
	}
	tracer.OnProduceRecordUnbuffered(record, nil)
	produced := p.Recorder.Ended()[0]
	if produced.Name() != "orders.tenant publish" || produced.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("native producer semantics")
	}
	record.Context = context.Background()
	tracer.OnFetchRecordBuffered(record)
	tracer.OnFetchRecordUnbuffered(record, false)
	if err = i.Process(ctx, record, func(context.Context, *kgo.Record) error { return errors.New("SECRET") }); err == nil {
		t.Fatal("handler error lost")
	}
	spans := p.Recorder.Ended()
	processed := spans[len(spans)-1]
	if processed.Name() != "orders.tenant process" || len(processed.Links()) != 1 || processed.Status().Code != codes.Error {
		t.Fatal("processing links/error semantics")
	}
	meter.OnBrokerConnect(kgo.BrokerMetadata{NodeID: 1}, 0, nil, nil)
	meter.OnBrokerWrite(kgo.BrokerMetadata{NodeID: 1}, 0, 128, 0, 0, nil)
	meter.OnProduceBatchWritten(kgo.BrokerMetadata{NodeID: 1}, "orders.tenant", 0, kgo.ProduceBatchMetrics{NumRecords: 1, UncompressedBytes: 20, CompressedBytes: 10})
	data := p.Collect(t)
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "messaging.kafka.write_bytes" {
				found = true
				if m.Unit != "by" {
					t.Fatal(m.Unit)
				}
				if m.Data.(metricdata.Sum[int64]).DataPoints[0].Value != 128 {
					t.Fatal("byte count")
				}
			}
		}
	}
	if !found {
		t.Fatalf("missing upstream byte metrics: %v", data)
	}
	if strings.Contains(fmt.Sprint(data), "orders.tenant") {
		t.Fatal("raw topic metric dimension")
	}
	parent.End()
	p.AssertPrivate(t, "SECRET")
}

func TestDirectKotelParityAndNativeOptions(t *testing.T) {
	var results [2][]string
	for index, adapter := range []bool{false, true} {
		p := testkit.New(t)
		formatter := func(*kgo.Record) (string, error) { return "safe-key", nil }
		var tracer *kotel.Tracer
		var meter *kotel.Meter
		if adapter {
			i, err := NewNative(p, WithRecordKeyFormatter(formatter), WithMergedConnectsMetrics(true))
			if err != nil {
				t.Fatal(err)
			}
			tracer = i.Hooks()[0].(*kotel.Tracer)
			meter = i.Hooks()[1].(*kotel.Meter)
		} else {
			tracer = kotel.NewTracer(kotel.TracerProvider(p.TP), kotel.TracerPropagator(p.Propagator), kotel.KeyFormatter(formatter))
			meter = kotel.NewMeter(kotel.MeterProvider(p.MP), kotel.WithMergedConnectsMeter())
		}
		record := &kgo.Record{Context: context.Background(), Topic: "orders", Key: []byte("SECRET"), Value: []byte("SECRET")}
		tracer.OnProduceRecordBuffered(record)
		tracer.OnProduceRecordUnbuffered(record, nil)
		record.Context = context.Background()
		tracer.OnFetchRecordBuffered(record)
		tracer.OnFetchRecordUnbuffered(record, false)
		_, s := tracer.WithProcessSpan(record)
		s.End()
		for _, span := range p.Recorder.Ended() {
			results[index] = append(results[index], fmt.Sprint(span.Name(), span.SpanKind(), len(span.Links())))
		}
		meter.OnBrokerConnect(kgo.BrokerMetadata{NodeID: 1}, 0, nil, errors.New("SECRET"))
		found := false
		for _, scope := range p.Collect(t).ScopeMetrics {
			for _, m := range scope.Metrics {
				if m.Name == "messaging.kafka.connect_errors.count" {
					t.Fatal("separate error counter still enabled")
				}
				if m.Name == "messaging.kafka.connects.count" {
					found = true
					data := m.Data.(metricdata.Sum[int64])
					if len(data.DataPoints) != 1 || data.DataPoints[0].Value != 1 {
						t.Fatal("native merged counter")
					}
					value, _ := data.DataPoints[0].Attributes.Value("outcome")
					if value.AsString() != "failure" {
						t.Fatal("outcome lost")
					}
				}
			}
		}
		if !found {
			t.Fatal("missing merged connect counter")
		}
		if adapter {
			if !strings.Contains(fmt.Sprint(p.Recorder.Ended()[0].Attributes()), "safe-key") {
				t.Fatal("safe formatted key lost")
			}
			p.AssertPrivate(t, "SECRET")
		}
	}
	if fmt.Sprint(results[0]) != fmt.Sprint(results[1]) {
		t.Fatal("native kotel span parity", results)
	}
	if _, err := New(nil, WithMergedConnectsMetrics(true)); err == nil {
		t.Fatal("native option silently ignored by legacy helpers")
	}
}
