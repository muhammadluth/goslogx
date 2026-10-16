//go:build integration

package compatibility

import (
	"context"
	goslogxsql "github.com/muhammadluth/goslogx/plugins/database/sql"
	"os"
	"testing"
	"time"
)

// Supply dedicated test database DSNs. These read-only smoke tests do not create schema.
func TestLiveSQLDrivers(t *testing.T) {
	for _, tc := range []struct{ driver, env string }{{"mysql", "GOSLOGX_TEST_MYSQL_DSN"}, {"postgres", "GOSLOGX_TEST_POSTGRES_DSN"}, {"sqlserver", "GOSLOGX_TEST_SQLSERVER_DSN"}} {
		t.Run(tc.driver, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skip("set " + tc.env + " to test a live server")
			}
			db, err := goslogxsql.Open(nil, tc.driver, dsn)
			if err != nil {
				t.Fatal("driver initialization failed")
			}
			defer db.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err = db.PingContext(ctx); err != nil {
				t.Fatal("database ping failed")
			}
			var one int
			if err = db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
				t.Fatal("database query failed")
			}
		})
	}
}
