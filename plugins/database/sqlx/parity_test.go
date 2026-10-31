package sqlx

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/XSAM/otelsql"
	native "github.com/jmoiron/sqlx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestDirectSQLXDriverParity(t *testing.T) {
	var spans [2][]string
	for index, adapter := range []bool{false, true} {
		p := testkit.New(t)
		name, _ := testkit.RegisterSQL(t)
		var db *native.DB
		var err error
		if adapter {
			db, err = Open(p, name, "", WithSpanOptions(SpanOptions{RowsNext: true, RowsChildOfQuery: true}))
		} else {
			var pool *sql.DB
			pool, err = otelsql.Open(name, "", otelsql.WithTracerProvider(p.TP), otelsql.WithMeterProvider(p.MP), otelsql.WithSQLCommenter(false), otelsql.WithSpanOptions(otelsql.SpanOptions{DisableQuery: true, RowsNext: true, RowsChildOfQuery: true}))
			if err == nil {
				db = native.NewDb(pool, name)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		var result int
		if err = db.GetContext(context.Background(), &result, "SELECT 1"); err != nil || result != 1 {
			t.Fatal("SQLX result", result, err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
		for _, s := range p.Recorder.Ended() {
			if s.SpanKind() != trace.SpanKindClient {
				t.Fatal("SQL span kind")
			}
			spans[index] = append(spans[index], fmt.Sprint(s.Name(), s.Status().Code, len(s.Events())))
		}
	}
	if fmt.Sprint(spans[0]) != fmt.Sprint(spans[1]) {
		t.Fatal("SQLX added or changed driver spans", spans)
	}
}
