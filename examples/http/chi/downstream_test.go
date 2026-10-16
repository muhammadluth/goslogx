package main

import (
	nativechi "github.com/go-chi/chi/v5"
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxchi "github.com/muhammadluth/goslogx/plugins/http/chi"
	"net/http"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "chi", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		m := nativechi.NewRouter()
		m.Use(goslogxchi.Middleware(p))
		m.Post("/orders/{id}", h.ServeHTTP)
		return m
	})
}
