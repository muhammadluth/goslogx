package compatibility

import (
	"context"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/microsoft/go-mssqldb"
	goslogxsql "github.com/muhammadluth/goslogx/plugins/database/sql"
	_ "modernc.org/sqlite"
	"testing"
)

func TestSQLiteQueriesAndTransactions(t *testing.T) {
	db, err := goslogxsql.Open(nil, "sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err = db.ExecContext(ctx, "CREATE TABLE orders (id INTEGER, status TEXT)"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO orders VALUES (?, ?)", 1, "SECRET"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var value string
	if err = db.QueryRowContext(ctx, "SELECT status FROM orders WHERE id = ?", 1).Scan(&value); err != nil || value != "SECRET" {
		t.Fatalf("query: %q %v", value, err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM orders"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders").Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback: %d %v", count, err)
	}
}

// Construction checks the real drivers without contacting an external server.
func TestDriverConstruction(t *testing.T) {
	for _, tt := range []struct{ name, dsn string }{{"mysql", "user:SECRET@tcp(localhost:3306)/orders"}, {"postgres", "postgres://user:SECRET@localhost/orders?sslmode=disable"}, {"sqlserver", "sqlserver://user:SECRET@localhost?database=orders"}} {
		t.Run(tt.name, func(t *testing.T) {
			db, err := goslogxsql.Open(nil, tt.name, tt.dsn)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
