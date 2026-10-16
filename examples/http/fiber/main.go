package main

import (
	"context"
	"github.com/gofiber/fiber/v3"
	"github.com/muhammadluth/goslogx"
	plugin "github.com/muhammadluth/goslogx/plugins/http/fiber"
)

func main() {
	logger := goslogx.New()
	defer logger.Shutdown(context.Background())
	app := fiber.New()
	app.Use(plugin.Middleware(logger))
	app.Get("/health", func(c fiber.Ctx) error { return c.SendString("ok") })
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
