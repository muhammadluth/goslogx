package main

import (
	nativegin "github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxgin "github.com/muhammadluth/goslogx/plugins/http/gin"
	"net/http"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "gin", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		nativegin.SetMode(nativegin.ReleaseMode)
		m := nativegin.New()
		m.Use(goslogxgin.Middleware(p))
		m.POST("/orders/:id", func(c *nativegin.Context) { h.ServeHTTP(c.Writer, c.Request) })
		return m
	})
}
