package main

import (
	"context"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
)

func TestSQLPluginExample(t *testing.T) {
	p := testkit.New(t)
	if err := run(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if len(p.Recorder.Ended()) == 0 || len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("plugin telemetry missing")
	}
	p.AssertPrivate(t, "CREATE TABLE orders", "INSERT INTO orders")
}
