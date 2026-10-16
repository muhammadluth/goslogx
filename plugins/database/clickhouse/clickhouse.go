// Package clickhouse instruments the native ClickHouse v2 API without inspecting values.
package clickhouse

import (
	"context"
	"errors"
	native "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"io"
	"strings"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/clickhouse"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type Conn struct {
	driver.Conn
	t *telemetry.Instrumentation
}

func Wrap(conn driver.Conn, p goslogx.TelemetryProvider, opts ...Option) (*Conn, error) {
	if telemetry.Nil(conn) {
		return nil, errors.New("clickhouse: nil connection")
	}
	if _, ok := conn.(*Conn); ok {
		return nil, errors.New("clickhouse: already instrumented")
	}
	t, err := telemetry.New(p, ScopeName, "clickhouse", false, opts...)
	if err != nil {
		return nil, err
	}
	return &Conn{conn, t}, nil
}
func (c *Conn) start(ctx context.Context, q string) (context.Context, func(error)) {
	op := "QUERY"
	if f := strings.Fields(q); len(f) > 0 {
		op = telemetry.DBOperation(f[0])
	}
	ctx, end := c.t.Begin(ctx, op, "", trace.SpanKindClient)
	c.t.Statement(ctx, q)
	return native.Context(ctx, native.WithSpan(trace.SpanContextFromContext(ctx))), end
}
func (c *Conn) Exec(ctx context.Context, q string, args ...any) error {
	ctx, end := c.start(ctx, q)
	err := c.Conn.Exec(ctx, q, args...)
	end(err)
	return err
}
func (c *Conn) Select(ctx context.Context, dest any, q string, args ...any) error {
	ctx, end := c.start(ctx, q)
	err := c.Conn.Select(ctx, dest, q, args...)
	end(err)
	return err
}
func (c *Conn) Query(ctx context.Context, q string, args ...any) (driver.Rows, error) {
	ctx, end := c.start(ctx, q)
	r, err := c.Conn.Query(ctx, q, args...)
	if err != nil {
		end(err)
		return r, err
	}
	return &rows{r, end}, nil
}
func (c *Conn) QueryRow(ctx context.Context, q string, args ...any) driver.Row {
	ctx, end := c.start(ctx, q)
	return &row{c.Conn.QueryRow(ctx, q, args...), end}
}
func (c *Conn) PrepareBatch(ctx context.Context, q string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	ctx, end := c.start(ctx, q)
	b, err := c.Conn.PrepareBatch(ctx, q, opts...)
	if err != nil {
		end(err)
		return b, err
	}
	return &batch{Batch: b, end: end}, nil
}
func (c *Conn) Ping(ctx context.Context) error {
	ctx, end := c.start(ctx, "PING")
	err := c.Conn.Ping(ctx)
	end(err)
	return err
}
func (c *Conn) AsyncInsert(ctx context.Context, q string, wait bool, args ...any) error {
	ctx, end := c.start(ctx, q)
	err := c.Conn.AsyncInsert(ctx, q, wait, args...)
	end(err)
	return err
}
func (c *Conn) InsertFormat(ctx context.Context, format, q string, data io.Reader) error {
	ctx, end := c.start(ctx, q)
	err := c.Conn.InsertFormat(ctx, format, q, data)
	end(err)
	return err
}
func (c *Conn) QueryFormat(ctx context.Context, format, q string, args ...any) (io.ReadCloser, error) {
	ctx, end := c.start(ctx, q)
	r, err := c.Conn.QueryFormat(ctx, format, q, args...)
	if err != nil {
		end(err)
		return r, err
	}
	return &stream{r, end}, nil
}

type rows struct {
	driver.Rows
	end func(error)
}

func (r *rows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.end(r.Rows.Err())
	}
	return ok
}
func (r *rows) Close() error {
	err := r.Rows.Close()
	if err == nil {
		err = r.Rows.Err()
	}
	r.end(err)
	return err
}
func (r *rows) Scan(v ...any) error {
	err := r.Rows.Scan(v...)
	if err != nil {
		r.end(err)
	}
	return err
}
func (r *rows) ScanStruct(v any) error {
	err := r.Rows.ScanStruct(v)
	if err != nil {
		r.end(err)
	}
	return err
}

type row struct {
	driver.Row
	end func(error)
}

func (r *row) Scan(v ...any) error    { err := r.Row.Scan(v...); r.end(err); return err }
func (r *row) ScanStruct(v any) error { err := r.Row.ScanStruct(v); r.end(err); return err }
func (r *row) Err() error {
	err := r.Row.Err()
	if err != nil {
		r.end(err)
	}
	return err
}

type batch struct {
	driver.Batch
	end func(error)
}

func (b *batch) Send() error  { err := b.Batch.Send(); b.end(err); return err }
func (b *batch) Abort() error { err := b.Batch.Abort(); b.end(err); return err }
func (b *batch) Close() error { err := b.Batch.Close(); b.end(err); return err }
func (b *batch) Append(v ...any) error {
	err := b.Batch.Append(v...)
	if err != nil {
		b.end(err)
	}
	return err
}
func (b *batch) AppendStruct(v any) error {
	err := b.Batch.AppendStruct(v)
	if err != nil {
		b.end(err)
	}
	return err
}
func (b *batch) Flush() error {
	err := b.Batch.Flush()
	if err != nil {
		b.end(err)
	}
	return err
}
func (b *batch) Column(i int) driver.BatchColumn { return &column{b.Batch.Column(i), b.end} }

type column struct {
	driver.BatchColumn
	end func(error)
}

func (c *column) Append(v any) error {
	err := c.BatchColumn.Append(v)
	if err != nil {
		c.end(err)
	}
	return err
}
func (c *column) AppendRow(v any) error {
	err := c.BatchColumn.AppendRow(v)
	if err != nil {
		c.end(err)
	}
	return err
}

type stream struct {
	io.ReadCloser
	end func(error)
}

func (s *stream) Read(b []byte) (int, error) {
	n, err := s.ReadCloser.Read(b)
	if err == io.EOF {
		s.end(nil)
	} else if err != nil {
		s.end(err)
	}
	return n, err
}
func (s *stream) Close() error { err := s.ReadCloser.Close(); s.end(err); return err }

// ObservePool observes local driver statistics; it never polls a server.
func (c *Conn) ObservePool() error {
	attrs := []attribute.KeyValue{attribute.String("db.system.name", "clickhouse")}
	if c.t.Config.Namespace != "" {
		attrs = append(attrs, attribute.String("db.namespace", c.t.Config.Namespace))
	}
	_, err := c.t.Meter.Int64ObservableUpDownCounter("db.client.connection.count", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		s := c.Stats()
		o.Observe(int64(s.Open-s.Idle), metric.WithAttributes(append(append([]attribute.KeyValue{}, attrs...), attribute.String("db.client.connection.state", "used"))...))
		o.Observe(int64(s.Idle), metric.WithAttributes(append(append([]attribute.KeyValue{}, attrs...), attribute.String("db.client.connection.state", "idle"))...))
		return nil
	}))
	if err != nil {
		return err
	}
	_, err = c.t.Meter.Int64ObservableUpDownCounter("db.client.connection.max", metric.WithUnit("{connection}"), metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
		o.Observe(int64(c.Stats().MaxOpenConns), metric.WithAttributes(attrs...))
		return nil
	}))
	return err
}

var _ driver.Conn = (*Conn)(nil)
