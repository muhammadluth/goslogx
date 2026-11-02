package sql

import (
	"context"
	dbsql "database/sql"
	"sync"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
)

var registerSystemDrivers sync.Once

// Use the public driver registration and Open paths: a driver's registration
// alias must not become a raw or ambiguous database-system metric dimension.
func TestRegisteredDriverSystemMetadata(t *testing.T) {
	registerSystemDrivers.Do(func() {
		for _, alias := range []string{"postgres", "mysql", "sqlite3", "mssql", "godror"} {
			dbsql.Register(alias, &testkit.SQLDriver{})
		}
	})
	for alias, want := range map[string]string{"postgres": "postgresql", "mysql": "mysql", "sqlite3": "sqlite", "mssql": "microsoft.sql_server", "godror": "oracle"} {
		t.Run(alias, func(t *testing.T) {
			p := testkit.New(t)
			db, err := Open(p, alias, "SECRET credentials")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.ExecContext(context.Background(), "SELECT 1"); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, s := range p.Recorder.Ended() {
				for _, a := range s.Attributes() {
					if a.Key == "db.system.name" {
						found = true
						if a.Value.AsString() != want {
							t.Fatal("driver system", a)
						}
					}
				}
			}
			if !found {
				t.Fatal("missing driver system")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
}
