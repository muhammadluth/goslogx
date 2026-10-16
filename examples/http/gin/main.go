package main

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/muhammadluth/goslogx"
	plugin "github.com/muhammadluth/goslogx/plugins/http/gin"
	"net/http"
)

func main() {
	logger := goslogx.New()
	defer logger.Shutdown(context.Background())
	app := gin.New()
	app.Use(plugin.Middleware(logger))
	app.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	if err := app.Run(":8080"); err != nil {
		panic(err)
	}
}
