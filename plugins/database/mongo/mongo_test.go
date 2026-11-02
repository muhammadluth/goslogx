package mongo

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"testing"
	"time"
)

func TestContribMonitorPrivacyAndPool(t *testing.T) {
	p := testkit.New(t)
	config := options.Client()
	if err := Instrument(config, p, WithDBName("payment")); err != nil {
		t.Fatal(err)
	}
	if Instrument(config, p) == nil {
		t.Fatal("existing monitors overwritten")
	}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	document, err := bson.Marshal(bson.D{{Key: "find", Value: "users"}, {Key: "filter", Value: bson.D{{Key: "password", Value: "SECRET"}}}})
	if err != nil {
		t.Fatal(err)
	}
	config.Monitor.Started(ctx, &event.CommandStartedEvent{Command: document, DatabaseName: "SECRET", CommandName: "find", RequestID: 1, ConnectionID: "mongo.internal:27017[-1]"})
	config.Monitor.Failed(ctx, &event.CommandFailedEvent{CommandFinishedEvent: event.CommandFinishedEvent{RequestID: 1, CommandName: "find", DatabaseName: "SECRET", ConnectionID: "mongo.internal:27017[-1]", Duration: time.Millisecond}, Failure: errors.New("SECRET")})
	config.PoolMonitor.Event(&event.PoolEvent{Type: event.ConnectionReady, Address: "SECRET", ConnectionID: 1})
	config.PoolMonitor.Event(&event.PoolEvent{Type: event.ConnectionCheckedOut, Address: "SECRET", ConnectionID: 1})
	config.PoolMonitor.Event(&event.PoolEvent{Type: event.ConnectionClosed, Address: "SECRET", ConnectionID: 1})
	parent.End()
	spans := p.Recorder.Ended()
	if len(spans) != 2 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("monitor trace parent")
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}
