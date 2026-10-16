package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/muhammadluth/goslogx"
	observed "github.com/muhammadluth/goslogx/plugins/messaging/nats"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := goslogx.New(goslogx.WithServiceName("nats-example"))
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = nats.DefaultURL
	}
	nc, err := nats.Connect(url)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	client, err := observed.Wrap(nc, logger, observed.WithDestinationNormalizer(func(string) string { return "orders" }))
	if err != nil {
		log.Fatal(err)
	}
	sub, err := nc.SubscribeSync("orders.created")
	if err != nil {
		log.Fatal(err)
	}
	defer sub.Unsubscribe()
	if err = nc.Flush(); err != nil {
		log.Fatal(err)
	}
	ctx, end := logger.StartTrace(context.Background(), "order.create")
	if err = client.Publish(ctx, "orders.created", []byte(`{"type":"order.created"}`)); err != nil {
		log.Fatal(err)
	}
	end()
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
	if err = client.Process(context.Background(), msg, func(ctx context.Context, _ *nats.Msg) error {
		logger.InfoContext(ctx, "", "orders-worker", goslogx.MESSSAGE_TYPE_EVENT, "event received", nil)
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
