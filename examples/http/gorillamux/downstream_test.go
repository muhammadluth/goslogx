package main

import (
	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxmux "github.com/muhammadluth/goslogx/plugins/http/gorillamux"
	"net/http"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "gorillamux", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		m := mux.NewRouter()
		m.Use(goslogxmux.Middleware(p))
		m.Handle("/orders/{id}", h).Methods("POST")
		return m
	})
}
