package main

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/muhammadluth/goslogx"
	plugin "github.com/muhammadluth/goslogx/plugins/http/chi"
	"net/http"
)

func main() {
	logger := goslogx.New()
	defer logger.Shutdown(context.Background())
	app := chi.NewRouter()
	app.Use(plugin.Middleware(logger))
	app.Get("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	if err := http.ListenAndServe(":8080", app); err != nil {
		panic(err)
	}
}
