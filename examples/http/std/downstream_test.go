package main

import (
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxhttp "github.com/muhammadluth/goslogx/plugins/http/std"
	"net/http"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "std", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		m := http.NewServeMux()
		m.Handle("POST /orders/{id}", h)
		return goslogxhttp.NewHandler(p, m, "api")
	})
}
