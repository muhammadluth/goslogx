package main

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/muhammadluth/goslogx"
	plugin "github.com/muhammadluth/goslogx/plugins/http/echo"
	"net/http"
)

func main() {
	logger := goslogx.New()
	defer logger.Shutdown(context.Background())
	app := echo.New()
	app.Use(plugin.Middleware(logger))
	app.GET("/health", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	if err := app.Start(":8080"); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
