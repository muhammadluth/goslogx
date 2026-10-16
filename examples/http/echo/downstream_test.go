package main

import (
	nativeecho "github.com/labstack/echo/v4"
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxecho "github.com/muhammadluth/goslogx/plugins/http/echo"
	"net/http"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "echo", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		m := nativeecho.New()
		m.Use(goslogxecho.Middleware(p))
		m.POST("/orders/:id", func(c nativeecho.Context) error { h.ServeHTTP(c.Response(), c.Request()); return nil })
		return m
	})
}
