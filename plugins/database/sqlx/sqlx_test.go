package sqlx

import (
	"context"
	"database/sql"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
)

func TestSQLXUsesInstrumentedDriver(t *testing.T) {
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	db, err := Open(p, name, "password=SECRET")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value int
	if err := db.GetContext(context.Background(), &value, "SELECT 'SECRET'"); err != nil || value != 1 {
		t.Fatalf("result %d %v", value, err)
	}
	if len(p.Recorder.Ended()) == 0 {
		t.Fatal("no spans")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestExistingPoolAndOpenFailure(t *testing.T) {
	if _, err := Open(nil, "missing-driver", ""); err == nil {
		t.Fatal("unknown driver accepted")
	}
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	pool, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := NewDb(pool, name)
	var value int
	if err = db.GetContext(context.Background(), &value, "SELECT 1"); err != nil || value != 1 {
		t.Fatal("native SQLX behavior", err)
	}
	if len(p.Recorder.Ended()) != 0 {
		t.Fatal("NewDb retrofitted telemetry onto raw pool")
	}
}
