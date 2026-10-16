package goslogx

import (
	"context"
)

func (l *Logger) ForceFlush(ctx context.Context) error {
	if l == nil || l.telemetry == nil {
		return nil
	}
	return l.telemetry.Lifecycle.ForceFlush(ctx)
}
func ForceFlush(ctx context.Context) error { return globalLog.Load().ForceFlush(ctx) }
func (l *Logger) Shutdown(ctx context.Context) error {
	if l == nil || l.telemetry == nil {
		return nil
	}
	return l.telemetry.Lifecycle.Shutdown(ctx)
}
func Shutdown(ctx context.Context) error { return globalLog.Load().Shutdown(ctx) }
