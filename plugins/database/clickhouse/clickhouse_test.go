package clickhouse

import (
	"context"
	"errors"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

type fakeConn struct {
	driver.Conn
	ctx context.Context
	err error
}

func (f *fakeConn) Exec(ctx context.Context, _ string, _ ...any) error { f.ctx = ctx; return f.err }
func (f *fakeConn) Query(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
	f.ctx = ctx
	return &fakeRows{}, nil
}
func (f *fakeConn) PrepareBatch(ctx context.Context, _ string, _ ...driver.PrepareBatchOption) (driver.Batch, error) {
	f.ctx = ctx
	return &fakeBatch{}, nil
}
func (f *fakeConn) Stats() driver.Stats { return driver.Stats{Open: 3, Idle: 1, MaxOpenConns: 10} }

type fakeRows struct{ driver.Rows }

func (*fakeRows) Next() bool   { return false }
func (*fakeRows) Err() error   { return nil }
func (*fakeRows) Close() error { return nil }

type fakeBatch struct{ driver.Batch }

func (*fakeBatch) Send() error  { return nil }
func (*fakeBatch) Close() error { return nil }
func TestNativeLifecycle(t *testing.T) {
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	f := &fakeConn{}
	c, err := Wrap(f, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ObservePool(); err != nil {
		t.Fatal(err)
	}
	if err = c.Exec(ctx, "INSERT SECRET", "SECRET"); err != nil {
		t.Fatal(err)
	}
	if !trace.SpanContextFromContext(f.ctx).IsValid() {
		t.Fatal("lost span")
	}
	r, err := c.Query(ctx, "SELECT SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recorder.Ended()) != 1 {
		t.Fatal("query ended before consumption")
	}
	r.Next()
	r.Close()
	b, err := c.PrepareBatch(ctx, "INSERT SECRET")
	if err != nil {
		t.Fatal(err)
	}
	b.Send()
	b.Close()
	f.err = errors.New("SECRET")
	if c.Exec(ctx, "UPDATE SECRET") != f.err {
		t.Fatal("error identity")
	}
	parent.End()
	if len(p.Recorder.Ended()) != 5 {
		t.Fatal(len(p.Recorder.Ended()))
	}
	p.AssertPrivate(t, "SECRET")
	if _, err = Wrap(c, p); err == nil {
		t.Fatal("duplicate accepted")
	}
}
