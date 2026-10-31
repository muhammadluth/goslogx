package pg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	native "github.com/go-pg/pg/v10"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
)

func TestNativeQueryCaptureMetadataAndSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			failure := errors.New("SECRET connection")
			db := native.Connect(&native.Options{Addr: "db.internal:5432", MaxRetries: 0, Dialer: func(context.Context, string, string) (net.Conn, error) { return nil, failure }})
			defer db.Close()
			err := Instrument(db, p, nil, WithTracing(signals[0]), WithMetrics(signals[1]), WithDBName("orders"), WithPoolName("primary"), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }), WithCallerEnabled(true), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")))
			if err != nil {
				t.Fatal(err)
			}
			if Instrument(db, p) == nil {
				t.Fatal("duplicate accepted")
			}
			if _, err = db.ExecContext(context.Background(), "SELECT ?", "SECRET"); !errors.Is(err, failure) {
				t.Fatal("native dial error", err)
			}
			spans := p.Recorder.Ended()
			if (len(spans) == 1) != signals[0] {
				t.Fatal("tracing mode")
			}
			if signals[0] {
				attrs := fmt.Sprint(spans[0].Attributes())
				for _, want := range []string{"db.internal", "5432", "SELECT ?", "code.function", "app.component"} {
					if !strings.Contains(attrs, want) {
						t.Fatal("lost metadata", want, attrs)
					}
				}
			}
			data := p.Collect(t)
			if (len(data.ScopeMetrics) > 0) != signals[1] {
				t.Fatal("metrics mode")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	p := testkit.New(t)
	db := native.Connect(&native.Options{MaxRetries: 0, Dialer: func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connection") }})
	defer db.Close()
	if err := Instrument(db, p, WithRawStatement(true)); err != nil {
		t.Fatal(err)
	}
	_, _ = db.ExecContext(context.Background(), "SELECT ?", "diagnostic")
	if spans := p.Recorder.Ended(); len(spans) != 1 || !strings.Contains(fmt.Sprint(spans[0].Attributes()), "diagnostic") {
		t.Fatal("explicit raw capture")
	}
}
