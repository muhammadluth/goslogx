package metrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecorderSignalsAndBoundedLabels(t *testing.T) {
	var disabled *Recorder
	disabled.RecordLog(nil, "SECRET")
	disabled.RecordTelemetryError(nil)
	disabled.RecordForceFlush(nil, 0, nil)
	disabled.RecordShutdown(nil, 0, nil)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	recorder := NewRecorder(mp)
	for _, severity := range []string{severityDebug, severityInfo, severityWarning, severityError, severityCritical, "SECRET"} {
		recorder.RecordLog(nil, severity)
	}
	recorder.RecordTelemetryError(nil)
	recorder.RecordForceFlush(nil, time.Millisecond, nil)
	recorder.RecordForceFlush(nil, time.Millisecond, errors.New("SECRET"))
	recorder.RecordShutdown(nil, time.Millisecond, nil)
	recorder.RecordShutdown(nil, time.Millisecond, errors.New("SECRET"))
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range data.ScopeMetrics {
		for _, m := range s.Metrics {
			names[m.Name] = true
		}
	}
	for _, name := range []string{metricLogRecords, metricTelemetryErrors, metricForceFlushDuration, metricForceFlushErrors, metricShutdownDuration, metricShutdownErrors} {
		if !names[name] {
			t.Error("missing instrument", name)
		}
	}
	if strings.Contains(fmt.Sprint(data), "SECRET") {
		t.Fatal("unbounded label or error")
	}
	if NewRecorder(nil) != nil {
		t.Fatal("nil provider must disable recorder")
	}
}
