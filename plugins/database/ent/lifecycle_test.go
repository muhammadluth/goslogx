package ent

import (
	"context"
	"entgo.io/ent/dialect"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/codes"
	"testing"
)

type configurableDriver struct {
	fakeDriver
	system    string
	failure   error
	txFailure error
}

func (d configurableDriver) Dialect() string                               { return d.system }
func (d configurableDriver) Exec(context.Context, string, any, any) error  { return d.failure }
func (d configurableDriver) Query(context.Context, string, any, any) error { return d.failure }
func (d configurableDriver) Tx(context.Context) (dialect.Tx, error) {
	return dialect.NopTx(d), d.txFailure
}

func TestTransactionsErrorsAndIndependentSignals(t *testing.T) {
	for _, system := range []string{dialect.MySQL, dialect.SQLite, "other"} {
		for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
			t.Run(fmt.Sprint(system, signals), func(t *testing.T) {
				p := testkit.New(t)
				d, err := WrapDriver(configurableDriver{system: system}, p, WithTracing(signals[0]), WithMetrics(signals[1]), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = WrapDriver(d, p); err == nil {
					t.Fatal("duplicate driver")
				}
				if err = d.Exec(context.Background(), "UPDATE SECRET", nil, nil); err != nil {
					t.Fatal(err)
				}
				tx, err := d.Tx(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if err = tx.Query(context.Background(), "SELECT SECRET", nil, nil); err != nil {
					t.Fatal(err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if (len(p.Recorder.Ended()) > 0) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
					t.Fatal("independent signals")
				}
				p.AssertPrivate(t, "SECRET")
			})
		}
	}
	if _, err := WrapDriver(nil, nil); err == nil {
		t.Fatal("nil driver")
	}
	if _, err := WrapDriver(fakeDriver{}, nil, WithStatementCapture(true)); err == nil {
		t.Fatal("missing sanitizer")
	}
	p := testkit.New(t)
	sentinel := errors.New("SECRET")
	d, err := WrapDriver(configurableDriver{failure: sentinel, txFailure: sentinel}, p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Exec(context.Background(), "", nil, nil) != sentinel {
		t.Fatal("native execution error")
	}
	if _, err = d.Tx(context.Background()); err != sentinel {
		t.Fatal("native transaction error")
	}
	for _, s := range p.Recorder.Ended() {
		if s.Status().Code != codes.Error || len(s.Events()) != 1 {
			t.Fatal("error classification")
		}
	}
	p.AssertPrivate(t, "SECRET")
}
