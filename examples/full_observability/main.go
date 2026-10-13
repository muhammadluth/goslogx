package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	goslogxbun "github.com/muhammadluth/goslogx/plugins/bun"
	goslogxfiber "github.com/muhammadluth/goslogx/plugins/fiber"
	goslogxhttp "github.com/muhammadluth/goslogx/plugins/http"
	goslogxredis "github.com/muhammadluth/goslogx/plugins/redis"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func newApp(logger *goslogx.Logger, db *bun.DB, cache redis.UniversalClient, client *http.Client, inventoryURL string) *fiber.App {
	app := fiber.New()
	app.Use(goslogxfiber.Middleware(logger, goslogxfiber.WithServerName("payment-api")))
	app.Post("/payments", func(c fiber.Ctx) error {
		ctx, span := logger.StartSpan(c.Context(), "payment.process")
		defer span.End()
		logger.InfoContext(ctx, "", "payment", goslogx.MESSSAGE_TYPE_REQUEST, "processing payment", nil)
		var one int
		if err := db.NewSelect().ColumnExpr("1").Scan(ctx, &one); err != nil {
			return fiber.NewError(503, "database unavailable")
		}
		if err := cache.Get(ctx, "inventory").Err(); err != nil && !errors.Is(err, redis.Nil) {
			return fiber.NewError(503, "cache unavailable")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, inventoryURL, nil)
		if err != nil {
			return fiber.NewError(503, "inventory unavailable")
		}
		res, err := client.Do(req)
		if err != nil {
			return fiber.NewError(503, "inventory unavailable")
		}
		_, readErr := io.Copy(io.Discard, res.Body)
		closeErr := res.Body.Close()
		if readErr != nil || closeErr != nil || res.StatusCode >= 400 {
			return fiber.NewError(503, "inventory unavailable")
		}
		return c.SendStatus(201)
	})
	return app
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	endpoint := env("OTEL_ENDPOINT", "localhost:4317")
	logger := goslogx.New(
		goslogx.WithServiceName("payment-api"),
		goslogx.WithResource(goslogx.ResourceConfig{ServiceVersion: "1.0", DeploymentEnvironment: "local", ServiceInstanceID: "payment-api-01"}),
		goslogx.WithTracing(goslogx.TracingConfig{Endpoint: endpoint, Insecure: true}),
		goslogx.WithMetrics(goslogx.MetricsConfig{Endpoint: endpoint, Insecure: true, ExportInterval: 5 * time.Second}),
		goslogx.WithInternalMetrics(true),
	)
	if err := errors.Join(logger.TracingError(), logger.MetricsError()); err != nil {
		log.Fatal(err)
	}
	db := bun.NewDB(sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(env("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")))), pgdialect.New())
	cache := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	closeMetrics := make(chan struct{})
	if err := errors.Join(goslogxbun.Instrument(db, logger, goslogxbun.WithDBName("payment")), goslogxredis.Instrument(cache, logger, goslogxredis.WithCloseChan(closeMetrics))); err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: goslogxhttp.NewTransport(logger, http.DefaultTransport)}
	app := newApp(logger, db, cache, client, env("INVENTORY_URL", "http://localhost:8081/health"))
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- app.Listen(":8080") }()
	select {
	case <-stop.Done():
	case err := <-serveErr:
		if err != nil {
			log.Print(err)
		}
	}
	ctx, timeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer timeout()
	_ = app.ShutdownWithContext(ctx)
	_ = cache.Close()
	close(closeMetrics)
	_ = db.Close()
	client.CloseIdleConnections()
	if err := logger.ForceFlush(ctx); err != nil {
		log.Print(err)
	}
	if err := logger.Shutdown(ctx); err != nil {
		log.Print(err)
	}
}
