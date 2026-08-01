package main

import (
	"context"

	"github.com/muhammadluth/goslogx"
)

func main() {
	goslogx.New(
		goslogx.WithServiceName("payment-service"),
		goslogx.WithMasking(true),
		// Use WithOTLPTracing("http://alloy:4317") in production.
		goslogx.WithTracing(),
	)
	goslogx.InfoContext(
		context.Background(),
		"payment-handler",
		goslogx.MESSSAGE_TYPE_EVENT,
		"payment received",
		map[string]any{"status": "received"},
	)
}
