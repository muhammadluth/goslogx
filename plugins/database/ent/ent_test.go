package ent

import (
	"context"
	"entgo.io/ent/dialect"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
)

type fakeDriver struct{}

func (fakeDriver) Exec(context.Context, string, any, any) error  { return nil }
func (fakeDriver) Query(context.Context, string, any, any) error { return nil }
func (d fakeDriver) Tx(context.Context) (dialect.Tx, error)      { return dialect.NopTx(d), nil }
func (fakeDriver) Close() error                                  { return nil }
func (fakeDriver) Dialect() string                               { return dialect.Postgres }
func TestDriverQueriesAndTransaction(t *testing.T) {
	p := testkit.New(t)
	driver, err := WrapDriver(fakeDriver{}, p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	if err := driver.Query(ctx, "SELECT 'SECRET'", []any{"SECRET"}, nil); err != nil {
		t.Fatal(err)
	}
	tx, err := driver.Tx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(ctx, "INSERT SECRET", []any{"SECRET"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	parent.End()
	spans := p.Recorder.Ended()
	if len(spans) != 4 || spans[1].Parent().SpanID() != spans[2].SpanContext().SpanID() {
		t.Fatalf("transaction chain: %v", spans)
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("metrics")
	}
}
