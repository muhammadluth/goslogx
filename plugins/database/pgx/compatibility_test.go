package pgx

import (
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"testing"
)

// Both directions must compile: these options were historically type aliases.
var _ Option = telemetry.WithTracing(false)
var _ telemetry.Option = WithMetrics(false)
var _ []Option = []telemetry.Option{WithTracing(false), WithMetrics(false)}

func TestHistoricalCommonOptionCompatibility(t *testing.T) {
	var c config
	options := []telemetry.Option{WithTracing(false), WithMetrics(false)}
	telemetry.ApplyOptions(&c.Config, &c, options...)
	if !c.TracingDisabled || !c.MetricsDisabled {
		t.Fatal("historical shared options lost")
	}
}
