package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/codes"
)

type lifecycleConn struct {
	fakeConn
	rows   *lifecycleRows
	batch  *lifecycleBatch
	row    *lifecycleRow
	stream io.ReadCloser
}

func (f *lifecycleConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	return f.rows, f.err
}
func (f *lifecycleConn) QueryRow(context.Context, string, ...any) driver.Row { return f.row }
func (f *lifecycleConn) PrepareBatch(context.Context, string, ...driver.PrepareBatchOption) (driver.Batch, error) {
	return f.batch, f.err
}
func (f *lifecycleConn) Select(context.Context, any, string, ...any) error             { return f.err }
func (f *lifecycleConn) Ping(context.Context) error                                    { return f.err }
func (f *lifecycleConn) AsyncInsert(context.Context, string, bool, ...any) error       { return f.err }
func (f *lifecycleConn) InsertFormat(context.Context, string, string, io.Reader) error { return f.err }
func (f *lifecycleConn) QueryFormat(context.Context, string, string, ...any) (io.ReadCloser, error) {
	return f.stream, f.err
}

type lifecycleRows struct {
	driver.Rows
	scanErr, iterationErr, closeErr error
	remaining                       int
}

func (r *lifecycleRows) Next() bool           { r.remaining--; return r.remaining >= 0 }
func (r *lifecycleRows) Err() error           { return r.iterationErr }
func (r *lifecycleRows) Scan(...any) error    { return r.scanErr }
func (r *lifecycleRows) ScanStruct(any) error { return r.scanErr }
func (r *lifecycleRows) Close() error         { return r.closeErr }

type lifecycleRow struct {
	driver.Row
	err error
}

func (r *lifecycleRow) Scan(...any) error    { return r.err }
func (r *lifecycleRow) ScanStruct(any) error { return r.err }
func (r *lifecycleRow) Err() error           { return r.err }

type lifecycleBatch struct {
	driver.Batch
	err error
}

func (b *lifecycleBatch) Append(...any) error           { return b.err }
func (b *lifecycleBatch) AppendStruct(any) error        { return b.err }
func (b *lifecycleBatch) Send() error                   { return b.err }
func (b *lifecycleBatch) Close() error                  { return b.err }
func (b *lifecycleBatch) Abort() error                  { return b.err }
func (b *lifecycleBatch) Flush() error                  { return b.err }
func (b *lifecycleBatch) Column(int) driver.BatchColumn { return &lifecycleColumn{err: b.err} }

type lifecycleColumn struct {
	driver.BatchColumn
	err error
}

func (c *lifecycleColumn) Append(any) error    { return c.err }
func (c *lifecycleColumn) AppendRow(any) error { return c.err }

type failedStream struct{ err error }

func (s failedStream) Read([]byte) (int, error) { return 0, s.err }
func (s failedStream) Close() error             { return s.err }

func TestCursorAndBatchErrorsWaitForCompletion(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("SECRET scan failure")
	for _, finish := range []string{"exhaust", "close"} {
		t.Run(finish, func(t *testing.T) {
			p := testkit.New(t)
			f := &lifecycleConn{rows: &lifecycleRows{scanErr: failure, remaining: 1}}
			c, err := Wrap(f, p)
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Query(ctx, "SELECT SECRET")
			if err != nil {
				t.Fatal(err)
			}
			if !r.Next() || r.Scan(new(int)) != failure || r.ScanStruct(new(int)) != failure {
				t.Fatal("native scan behavior")
			}
			if len(p.Recorder.Ended()) != 0 {
				t.Fatal("scan failure prematurely ended cursor")
			}
			f.rows.iterationErr = failure
			if finish == "exhaust" {
				if r.Next() {
					t.Fatal("unexpected row")
				}
			} else if err = r.Close(); err != nil {
				t.Fatal("Close must retain native nil return")
			}
			_ = r.Close()
			if spans := p.Recorder.Ended(); len(spans) != 1 || spans[0].Status().Code != codes.Error {
				t.Fatal("cursor completion telemetry")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	for _, finish := range []string{"send", "abort", "close"} {
		t.Run(finish, func(t *testing.T) {
			p := testkit.New(t)
			f := &lifecycleConn{batch: &lifecycleBatch{err: failure}}
			c, _ := Wrap(f, p)
			b, err := c.PrepareBatch(ctx, "INSERT SECRET")
			if err != nil {
				t.Fatal(err)
			}
			for _, err := range []error{b.Append("SECRET"), b.AppendStruct(nil), b.Flush(), b.Column(0).Append(nil), b.Column(0).AppendRow(nil)} {
				if err != failure {
					t.Fatal("append error changed")
				}
			}
			if len(p.Recorder.Ended()) != 0 {
				t.Fatal("batch ended before completion")
			}
			f.batch.err = nil
			switch finish {
			case "send":
				err = b.Send()
			case "abort":
				err = b.Abort()
			case "close":
				err = b.Close()
			}
			if err != nil {
				t.Fatal("terminal native error changed")
			}
			_ = b.Close()
			if spans := p.Recorder.Ended(); len(spans) != 1 || spans[0].Status().Code != codes.Error {
				t.Fatal("batch failure lost")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
}

func TestRowsStreamsAndNativeOperations(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("SECRET operation")
	for _, nativeErr := range []error{nil, failure} {
		t.Run(fmt.Sprint(nativeErr == nil), func(t *testing.T) {
			p := testkit.New(t)
			f := &lifecycleConn{row: &lifecycleRow{err: nativeErr}, rows: &lifecycleRows{closeErr: nativeErr}, batch: &lifecycleBatch{}, stream: io.NopCloser(strings.NewReader("data"))}
			f.err = nativeErr
			c, err := Wrap(f, p, WithDBName("warehouse"))
			if err != nil {
				t.Fatal(err)
			}
			for _, err := range []error{c.Select(ctx, nil, "SELECT SECRET"), c.Ping(ctx), c.AsyncInsert(ctx, "INSERT SECRET", false), c.InsertFormat(ctx, "CSV", "INSERT SECRET", strings.NewReader("SECRET"))} {
				if err != nativeErr {
					t.Fatal("operation return changed")
				}
			}
			for _, scan := range []bool{true, false} {
				r := c.QueryRow(ctx, "SELECT SECRET")
				if r.Err() != nativeErr {
					t.Fatal("row error changed")
				}
				if scan {
					err = r.Scan(new(int))
				} else {
					err = r.ScanStruct(new(int))
				}
				if err != nativeErr {
					t.Fatal("row scan error changed")
				}
			}
			if nativeErr != nil {
				if _, err = c.Query(ctx, "SELECT SECRET"); err != nativeErr {
					t.Fatal(err)
				}
				if _, err = c.PrepareBatch(ctx, "INSERT SECRET"); err != nativeErr {
					t.Fatal(err)
				}
				if _, err = c.QueryFormat(ctx, "JSON", "SELECT SECRET"); err != nativeErr {
					t.Fatal(err)
				}
			}
			f.err = nil
			if nativeErr != nil {
				f.stream = failedStream{nativeErr}
			}
			r, err := c.QueryFormat(ctx, "JSON", "SELECT SECRET")
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.ReadAll(r)
			if err != nativeErr {
				t.Fatal(err)
			}
			if err = r.Close(); err != nativeErr {
				t.Fatal(err)
			}
			rows, err := c.Query(ctx, "SELECT SECRET")
			if err != nil {
				t.Fatal(err)
			}
			if rows.Close() != nativeErr {
				t.Fatal("close error changed")
			}
			if err = c.ObservePool(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(fmt.Sprint(p.Collect(t)), "db.client.connection.max") {
				t.Fatal("pool metrics absent")
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	if _, err := Wrap(nil, nil); err == nil {
		t.Fatal("nil accepted")
	}
	if _, err := Wrap(&fakeConn{}, nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe capture accepted")
	}
}
