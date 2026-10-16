package main

import (
	nativefiber "github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	testkit "github.com/muhammadluth/goslogx/examples/http/internal/testkit"
	goslogxfiber "github.com/muhammadluth/goslogx/plugins/http/fiber"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownstream(t *testing.T) {
	testkit.RunDownstream(t, "fiber", func(p goslogx.TelemetryProvider, h http.Handler) http.Handler {
		m := nativefiber.New()
		m.Use(goslogxfiber.Middleware(p))
		m.Post("/orders/:id", func(c nativefiber.Ctx) error {
			r, _ := http.NewRequestWithContext(c.Context(), "POST", "http://localhost"+c.OriginalURL(), nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			c.Status(w.Code)
			return c.Send(w.Body.Bytes())
		})
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := m.Test(r)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			w.WriteHeader(res.StatusCode)
			io.Copy(w, res.Body)
		})
	})
}
