package sql

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
)

func TestDriverQueriesTransactionsPrivacy(t *testing.T) {
	p := testkit.New(t)
	name, d := testkit.RegisterSQL(t)
	db, err := Open(p, name, "password=SECRET", WithDBName("payment"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	var value int
	if err := db.QueryRowContext(ctx, "SELECT 'SECRET'", "SECRET").Scan(&value); err != nil || value != 1 {
		t.Fatalf("query: %d %v", value, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT SECRET", "SECRET"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("password=SECRET")
	d.ExecError = sentinel
	if _, err := db.ExecContext(ctx, "DELETE SECRET"); err != sentinel {
		t.Fatal("native error changed")
	}
	parent.End()
	if len(p.Recorder.Ended()) < 4 {
		t.Fatal("missing driver spans")
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("missing SQL metrics")
	}
}
func TestStatementSanitizer(t *testing.T) {
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	if _, err := Open(p, name, "", WithStatementCapture(true)); err == nil {
		t.Fatal("missing sanitizer accepted")
	}
	db, err := Open(p, name, "", WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(context.Background(), "SELECT 'SECRET'", "SECRET"); err != nil {
		t.Fatal(err)
	}
	p.AssertPrivate(t, "SECRET")
}
