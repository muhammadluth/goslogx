package testkit

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
)

// SQLDriver exercises real database/sql execution without a network service.
type SQLDriver struct{ ExecError, QueryError error }

var nextDriver atomic.Int64

func RegisterSQL(t testing.TB) (string, *SQLDriver) {
	t.Helper()
	name := fmt.Sprintf("goslogx_test_%d", nextDriver.Add(1))
	d := &SQLDriver{}
	sql.Register(name, d)
	return name, d
}
func (d *SQLDriver) Open(string) (driver.Conn, error)               { return sqlConn{d}, nil }
func (d *SQLDriver) OpenConnector(string) (driver.Connector, error) { return sqlConnector{d}, nil }

type sqlConnector struct{ d *SQLDriver }

func (c sqlConnector) Driver() driver.Driver                        { return c.d }
func (c sqlConnector) Connect(context.Context) (driver.Conn, error) { return c.d.Open("") }

type sqlConn struct{ d *SQLDriver }

func (c sqlConn) Prepare(string) (driver.Stmt, error) { return sqlStmt{c}, nil }
func (sqlConn) Close() error                          { return nil }
func (sqlConn) Begin() (driver.Tx, error)             { return sqlTx{}, nil }
func (sqlConn) Ping(context.Context) error            { return nil }
func (c sqlConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.d.ExecError != nil {
		return nil, c.d.ExecError
	}
	return driver.RowsAffected(1), nil
}
func (c sqlConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	if c.d.QueryError != nil {
		return nil, c.d.QueryError
	}
	return &sqlRows{}, nil
}

type sqlStmt struct{ c sqlConn }

func (sqlStmt) Close() error  { return nil }
func (sqlStmt) NumInput() int { return -1 }
func (s sqlStmt) Exec([]driver.Value) (driver.Result, error) {
	return s.c.ExecContext(context.Background(), "", nil)
}
func (s sqlStmt) Query([]driver.Value) (driver.Rows, error) {
	return s.c.QueryContext(context.Background(), "", nil)
}

type sqlTx struct{}

func (sqlTx) Commit() error   { return nil }
func (sqlTx) Rollback() error { return nil }

type sqlRows struct{ done bool }

func (*sqlRows) Columns() []string { return []string{"value"} }
func (*sqlRows) Close() error      { return nil }
func (r *sqlRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = int64(1)
	r.done = true
	return nil
}
