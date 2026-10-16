package sqlx

import (
	"context"
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
