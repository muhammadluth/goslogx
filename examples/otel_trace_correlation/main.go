package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/muhammadluth/goslogx"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logger := goslogx.New(
		goslogx.WithServiceName("payment-service"),
		goslogx.WithMasking(true),
		goslogx.WithTracing(goslogx.TracingConfig{
			Endpoint:              "127.0.0.1:4317",
			SamplingRatio:         0.1,
			Insecure:              true, // Local Collector from the Compose example.
			BatchTimeout:          5 * time.Second,
			ExportTimeout:         5 * time.Second,
			MaxQueueSize:          2048,
			MaxExportBatchSize:    512,
			ServiceVersion:        "1.4.2",
			DeploymentEnvironment: "development",
			ServiceInstanceID:     "payment-service-1",
			ResourceAttributes: map[string]string{
				"team.name": "payments",
			},
		}),
	)
	if err := logger.TracingError(); err != nil {
		log.Fatalf("initialize tracing: %v", err)
	}

	router := http.NewServeMux()
	router.HandleFunc("GET /payments/{id}", func(w http.ResponseWriter, request *http.Request) {
		paymentID := request.PathValue("id")
		logger.InfoContext(
			request.Context(),
			paymentID,
			"payment-handler",
			goslogx.MESSSAGE_TYPE_EVENT,
			"payment received",
			map[string]any{"payment_id": paymentID},
		)
		w.WriteHeader(http.StatusNoContent)
	})

	handler := otelhttp.NewHandler(
		router,
		"payment-api",
		otelhttp.WithTracerProvider(logger.TracerProvider()),
		otelhttp.WithPropagators(goslogx.TextMapPropagator()),
	)
	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	applicationCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
			stop()
		}
	}()

	<-applicationCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	if err := logger.ForceFlush(shutdownCtx); err != nil {
		log.Printf("telemetry flush: %v", err)
	}
	if err := logger.Shutdown(shutdownCtx); err != nil {
		log.Printf("telemetry shutdown: %v", err)
	}
}
