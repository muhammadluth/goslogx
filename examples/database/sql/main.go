// A schema-free local SQLite example using the optional SQL plugin.
package main

import (
	"context"
	"log"

	"github.com/muhammadluth/goslogx"
	goslogxsql "github.com/muhammadluth/goslogx/plugins/database/sql"
	_ "modernc.org/sqlite"
)

func run(ctx context.Context, p goslogx.TelemetryProvider) error {
	db, err := goslogxsql.Open(p, "sqlite", ":memory:", goslogxsql.WithDBName("orders"))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "CREATE TABLE orders (id INTEGER, status TEXT)"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO orders VALUES (?, ?)", 1, "created"); err != nil {
		return err
	}
	return tx.Commit()
}
func main() {
	logger := goslogx.New(goslogx.WithServiceName("sql-example"))
	if err := run(context.Background(), logger); err != nil {
		log.Fatal(err)
	}
}
