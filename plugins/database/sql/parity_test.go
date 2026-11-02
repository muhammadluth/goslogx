package sql

import (
	"context"
	dbsql "database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/XSAM/otelsql"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

func TestDriverLifecycleOptionsAndNativeParity(t *testing.T) {
	p := testkit.New(t)
	name, d := testkit.RegisterSQL(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	defer parent.End()
	for _, direct := range []bool{true, false} {
		var db *dbsql.DB
		var err error
		if direct {
			db, err = otelsql.Open(name, "", otelsql.WithTracerProvider(p.TP), otelsql.WithMeterProvider(p.MP), otelsql.WithSpanOptions(otelsql.SpanOptions{Ping: true, RowsNext: true, RowsChildOfQuery: true, DisableQuery: true}))
		} else {
			db, err = Open(p, name, "", WithSpanOptions(SpanOptions{Ping: true, RowsNext: true, RowsChildOfQuery: true}), WithTraceAttributes(attribute.String("app.component", "store")), WithMetricAttributes(attribute.String("app.region", "test")), WithPropagator(propagation.TraceContext{}))
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = db.PingContext(ctx); err != nil {
			t.Fatal(err)
		}
		stmt, err := db.PrepareContext(ctx, "SELECT ?")
		if err != nil {
			t.Fatal(err)
		}
		rows, err := stmt.QueryContext(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value int
			if err = rows.Scan(&value); err != nil || value != 1 {
				t.Fatal("scan", err)
			}
		}
		if err = rows.Close(); err != nil {
			t.Fatal(err)
		}
		if err = stmt.Close(); err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE users SET status=?", 1); err != nil {
			t.Fatal(err)
		}
		if err = tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	directNames, adapterNames := map[string]int{}, map[string]int{}
	directEvents, adapterEvents := 0, 0
	for _, span := range p.Recorder.Ended() {
		direct := span.InstrumentationScope().Name != ScopeName
		if direct {
			directNames[span.Name()]++
		} else {
			adapterNames[span.Name()]++
		}
		for _, event := range span.Events() {
			if event.Name == "sql.rows.next" {
				if direct {
					directEvents++
				} else {
					adapterEvents++
				}
			}
		}
	}
	if fmt.Sprint(directNames) != fmt.Sprint(adapterNames) || directEvents == 0 || directEvents != adapterEvents {
		t.Fatal("native lifecycle/event parity", directNames, adapterNames, directEvents, adapterEvents)
	}
	d.ExecError = errors.New("SECRET")
	db, err := Open(p, name, "", WithFilter(func(_ context.Context, method, query string, _ []driver.NamedValue) bool {
		return method != "sql.conn.exec"
	}), WithSpanNameFormatter(func(_ context.Context, method, _ string) string { return "native " + method }), WithSQLCommenter(true), WithRawStatement(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, "UPDATE users SET status=?", 1); err != d.ExecError {
		t.Fatal("filtered native error")
	}
	for _, span := range p.Recorder.Ended() {
		if span.Name() == "native sql.conn.exec" {
			t.Fatal("filter ignored")
		}
	}
}

func TestConnectorsWrappedDriversAndIndependentSignals(t *testing.T) {
	for _, signals := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprint(signals), func(t *testing.T) {
			p := testkit.New(t)
			d := &testkit.SQLDriver{}
			wrapped, err := WrapDriver(p, d, WithTracing(signals[0]), WithMetrics(signals[1]), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }), WithDBName("orders"))
			if err != nil {
				t.Fatal(err)
			}
			connector, err := wrapped.(driver.DriverContext).OpenConnector("")
			if err != nil {
				t.Fatal(err)
			}
			db := dbsql.OpenDB(connector)
			defer db.Close()
			if _, err = db.ExecContext(context.Background(), "SELECT SECRET"); err != nil {
				t.Fatal(err)
			}
			if (len(p.Recorder.Ended()) > 0) != signals[0] || (len(p.Collect(t).ScopeMetrics) > 0) != signals[1] {
				t.Fatal("signal modes")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	p := testkit.New(t)
	d := &testkit.SQLDriver{}
	connector, _ := d.OpenConnector("")
	db, err := OpenDB(p, connector, WithRawStatement(true), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	query := strings.Repeat("x", 4100)
	if _, err = db.ExecContext(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	for _, span := range p.Recorder.Ended() {
		for _, a := range span.Attributes() {
			if a.Key == "db.query.text" && len(a.Value.AsString()) != 4096 {
				t.Fatal("capture length")
			}
		}
	}
	if _, err = Open(nil, "unregistered", ""); err == nil {
		t.Fatal("unknown driver")
	}
	if _, err = OpenDB(nil, nil); err == nil {
		t.Fatal("nil connector")
	}
	if _, err = WrapDriver(nil, nil); err == nil {
		t.Fatal("nil driver")
	}
	if _, err = OpenDB(nil, connector, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe connector capture")
	}
	if _, err = WrapDriver(nil, d, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe driver capture")
	}
}
