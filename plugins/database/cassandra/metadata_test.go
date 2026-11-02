package cassandra

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	native "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
)

func TestHostRowsStatementAndSignalModes(t *testing.T) {
	host, err := native.NewHostInfoFromAddrPort(net.ParseIP("127.0.0.1"), 9042)
	if err != nil {
		t.Fatal(err)
	}
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			o, err := New(p, WithTracing(signals[0]), WithMetrics(signals[1]), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")))
			if err != nil {
				t.Fatal(err)
			}
			o.ObserveQuery(nil, native.ObservedQuery{Statement: "SELECT SECRET", Rows: 3, Host: host})
			start := time.Now()
			o.ObserveBatch(context.Background(), native.ObservedBatch{Start: start, End: start.Add(-time.Second), Statements: []string{"SECRET", "SECRET"}, Host: host})
			spans := p.Recorder.Ended()
			if (len(spans) == 2) != signals[0] {
				t.Fatal("signal mode")
			}
			if signals[0] {
				attrs := fmt.Sprint(spans[0].Attributes())
				for _, want := range []string{"127.0.0.1", "9042", "db.response.returned_rows", "SELECT ?", "app.component"} {
					if !strings.Contains(attrs, want) {
						t.Fatal("metadata lost", want, attrs)
					}
				}
				if spans[1].EndTime().Before(spans[1].StartTime()) {
					t.Fatal("negative duration")
				}
			}
			if (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("metrics mode")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	if InstrumentConfig(nil, nil) == nil {
		t.Fatal("nil config")
	}
	if _, err := New(nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe capture")
	}
	if err := InstrumentConfig(native.NewCluster("localhost"), nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe config capture")
	}
	o, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	o.ObserveQuery(nil, native.ObservedQuery{Statement: ""})
}
