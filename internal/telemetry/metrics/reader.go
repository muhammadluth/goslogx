package metrics

import (
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func NewReader(exporter sdkmetric.Exporter, interval, timeout time.Duration) *sdkmetric.PeriodicReader {
	return sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval), sdkmetric.WithTimeout(timeout))
}
