package mongo

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/v2/mongo/otelmongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

func TestNativeMonitorParityAndSignalModes(t *testing.T) {
	document, err := bson.Marshal(bson.D{{Key: "find", Value: "users"}, {Key: "filter", Value: bson.D{{Key: "password", Value: "SECRET"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			m, err := NewMonitors(p, nil, WithTracing(signals[0]), WithMetrics(signals[1]), WithDBName("orders"), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "find users" }), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")), WithPropagator(propagation.TraceContext{}), WithSpanNameFormatter(func(e *event.CommandStartedEvent) string { return "lookup " + e.DatabaseName }))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			m.Command.Started(ctx, &event.CommandStartedEvent{Command: document, DatabaseName: "actual", CommandName: "find", RequestID: 1, ConnectionID: "mongo:27017[-1]"})
			m.Command.Succeeded(ctx, &event.CommandSucceededEvent{CommandFinishedEvent: event.CommandFinishedEvent{DatabaseName: "actual", CommandName: "find", RequestID: 1, ConnectionID: "mongo:27017[-1]", Duration: time.Millisecond}})
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] {
				t.Fatal("tracing mode")
			}
			if signals[0] && (spans[0].Name() != "lookup orders" || !strings.Contains(fmt.Sprint(spans[0].Attributes()), "find users")) {
				t.Fatal("formatter/statement policy", spans[0].Attributes())
			}
			for _, kind := range []string{event.ConnectionCreated, event.ConnectionReady, event.ConnectionReady, event.ConnectionCheckedOut, event.ConnectionCheckedIn, event.ConnectionPoolClosed, event.ConnectionClosed} {
				m.Pool.Event(&event.PoolEvent{Type: kind, Address: "mongo:27017", ConnectionID: 1})
			}
			if (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("metrics mode")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	direct := otelmongo.NewMonitor(otelmongo.WithTracerProvider(p.TP), otelmongo.WithMeterProvider(p.MP), otelmongo.WithCommandAttributeDisabled(true))
	m, err := NewMonitors(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, monitor := range []*event.CommandMonitor{direct, m.Command} {
		monitor.Started(ctx, &event.CommandStartedEvent{Command: document, CommandName: "find", DatabaseName: "orders", RequestID: 2, ConnectionID: "mongo:27017[-1]"})
		monitor.Succeeded(ctx, &event.CommandSucceededEvent{CommandFinishedEvent: event.CommandFinishedEvent{CommandName: "find", DatabaseName: "orders", RequestID: 2, ConnectionID: "mongo:27017[-1]", Duration: time.Millisecond}})
	}
	a, b := p.Recorder.Ended()[0], p.Recorder.Ended()[1]
	if a.Name() != b.Name() || a.SpanKind() != b.SpanKind() || !a.Parent().Equal(b.Parent()) || a.Status() != b.Status() {
		t.Fatal("native span parity")
	}
	p.AssertPrivate(t, "SECRET")
	if Instrument(nil, nil) == nil {
		t.Fatal("nil config")
	}
	if _, err := NewMonitors(nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe capture")
	}
	if _, err := NewMonitors(nil, WithRawStatement(true)); err != nil {
		t.Fatal(err)
	}
}
