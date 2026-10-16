package lifecycle

import (
	"context"
	"sync"
	"time"

	metrics "github.com/muhammadluth/goslogx/internal/telemetry/metrics"
	tracing "github.com/muhammadluth/goslogx/internal/telemetry/tracing"
	"go.uber.org/zap"
)

type ErrorHandler struct {
	logger          *zap.Logger
	internalMetrics *metrics.Recorder
	interval        time.Duration
	mu              sync.Mutex
	lastLogged      time.Time
	suppressed      uint64
}

func NewErrorHandler(logger *zap.Logger, interval time.Duration, internalMetrics *metrics.Recorder) *ErrorHandler {
	if interval == 0 {
		interval = tracing.DefaultOTelErrorLogInterval
	}
	return &ErrorHandler{
		logger:          logger,
		internalMetrics: internalMetrics,
		interval:        interval,
	}
}
func (h *ErrorHandler) Handle(err error) {
	if h == nil || h.logger == nil || err == nil {
		return
	}
	h.internalMetrics.RecordTelemetryError(context.Background())

	h.mu.Lock()
	now := time.Now()
	if !h.lastLogged.IsZero() && now.Sub(h.lastLogged) < h.interval {
		h.suppressed++
		h.mu.Unlock()
		return
	}
	suppressed := h.suppressed
	h.suppressed = 0
	h.lastLogged = now
	h.mu.Unlock()

	h.logger.Error("OpenTelemetry internal error",
		zap.Error(err),
		zap.Uint64("suppressed_errors", suppressed),
	)
}
