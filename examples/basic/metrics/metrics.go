// Package otelmetrics demonstrates plugin HTTP metrics alongside a business metric.
package otelmetrics

import (
	"net/http"

	"github.com/muhammadluth/goslogx"
	observed "github.com/muhammadluth/goslogx/plugins/http/std"
	"go.opentelemetry.io/otel/metric"
)

func NewHandler(logger *goslogx.Logger, next http.Handler) (http.Handler, error) {
	created, err := logger.Meter("example.orders").Int64Counter("orders.created", metric.WithUnit("{order}"))
	if err != nil {
		return nil, err
	}
	return observed.NewHandler(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { created.Add(r.Context(), 1); next.ServeHTTP(w, r) }), "orders", observed.WithRoute("/orders/{id}")), nil
}
