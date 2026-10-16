package main

import (
	"context"
	"github.com/gorilla/mux"
	"github.com/muhammadluth/goslogx"
	plugin "github.com/muhammadluth/goslogx/plugins/http/gorillamux"
	"net/http"
)

func main() {
	logger := goslogx.New()
	defer logger.Shutdown(context.Background())
	app := mux.NewRouter()
	app.Use(plugin.Middleware(logger))
	app.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }).Methods("GET")
	if err := http.ListenAndServe(":8080", app); err != nil {
		panic(err)
	}
}
