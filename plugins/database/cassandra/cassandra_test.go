package cassandra

import (
	"context"
	"errors"
	native "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
	"time"
)

func TestObserverTimingAndPrivacy(t *testing.T) {
	p := testkit.New(t)
	cfg := native.NewCluster("SECRET")
	if err := InstrumentConfig(cfg, p, WithDBName("orders")); err != nil {
		t.Fatal(err)
	}
	if InstrumentConfig(cfg, p) == nil {
		t.Fatal("duplicate accepted")
	}
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	start := time.Now().Add(-time.Second)
	end := start.Add(time.Millisecond * 12)
	cfg.QueryObserver.ObserveQuery(ctx, native.ObservedQuery{Statement: "SELECT SECRET", Values: []any{"SECRET"}, Start: start, End: end, Err: errors.New("SECRET")})
	cfg.BatchObserver.ObserveBatch(ctx, native.ObservedBatch{Statements: []string{"SECRET"}, Start: start, End: end})
	o := cfg.QueryObserver.(*Observer)
	sentinel := errors.New("SECRET")
	if err := o.Iter(ctx, func(context.Context) error { return sentinel }); err != sentinel {
		t.Fatal("error identity")
	}
	parent.End()
	spans := p.Recorder.Ended()
	if len(spans) != 4 || spans[0].EndTime().Sub(spans[0].StartTime()) != 12*time.Millisecond {
		t.Fatal("timing")
	}
	if spans[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("parent")
	}
	p.AssertPrivate(t, "SECRET")
}
