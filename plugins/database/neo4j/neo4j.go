// Package neo4j instruments official driver v6 sessions and transactions.
package neo4j

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	native "github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"go.opentelemetry.io/otel/trace"
	"sync"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/neo4j"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithStatementCapture = telemetry.WithStatementCapture
var WithStatementSanitizer = telemetry.WithStatementSanitizer
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

type Session struct {
	native.Session
	t       *telemetry.Instrumentation
	mu      sync.Mutex
	pending map[*result]struct{}
}

func WrapSession(s native.Session, p goslogx.TelemetryProvider, opts ...Option) (*Session, error) {
	if telemetry.Nil(s) {
		return nil, errors.New("neo4j: nil session")
	}
	if _, ok := s.(*Session); ok {
		return nil, errors.New("neo4j: already instrumented")
	}
	t, err := telemetry.New(p, ScopeName, "neo4j", false, opts...)
	if err != nil {
		return nil, err
	}
	return &Session{Session: s, t: t, pending: make(map[*result]struct{})}, nil
}
func (s *Session) query(ctx context.Context, q string, fn func(context.Context) (native.Result, error)) (native.Result, error) {
	ctx, end := s.t.Begin(ctx, "RUN", "", trace.SpanKindClient)
	s.t.Statement(ctx, q)
	r, err := fn(ctx)
	if err != nil {
		end(err)
		return r, err
	}
	wrapped := &result{Result: r}
	var once sync.Once
	wrapped.end = func(err error) { once.Do(func() { end(err); s.mu.Lock(); delete(s.pending, wrapped); s.mu.Unlock() }) }
	s.mu.Lock()
	s.pending[wrapped] = struct{}{}
	s.mu.Unlock()
	return wrapped, nil
}
func (s *Session) Run(ctx context.Context, q string, params map[string]any, opts ...func(*native.TransactionConfig)) (native.Result, error) {
	return s.query(ctx, q, func(ctx context.Context) (native.Result, error) { return s.Session.Run(ctx, q, params, opts...) })
}
func (s *Session) finishPending(err error) {
	s.mu.Lock()
	pending := s.pending
	s.pending = make(map[*result]struct{})
	s.mu.Unlock()
	for r := range pending {
		r.end(err)
	}
}
func (s *Session) Close(ctx context.Context) error {
	return s.t.Run(ctx, "CLOSE", trace.SpanKindInternal, func(ctx context.Context) error { err := s.Session.Close(ctx); s.finishPending(err); return err })
}
func (s *Session) BeginTransaction(ctx context.Context, opts ...func(*native.TransactionConfig)) (native.ExplicitTransaction, error) {
	ctx, end := s.t.Begin(ctx, "BEGIN", "", trace.SpanKindClient)
	tx, err := s.Session.BeginTransaction(ctx, opts...)
	if err != nil {
		end(err)
		return tx, err
	}
	return &transaction{ExplicitTransaction: tx, s: s, ctx: ctx, end: end}, nil
}
func (s *Session) managed(ctx context.Context, write bool, work native.ManagedTransactionWork, opts ...func(*native.TransactionConfig)) (out any, err error) {
	err = s.t.Run(ctx, "BEGIN", trace.SpanKindClient, func(ctx context.Context) error {
		fn := func(tx native.ManagedTransaction) (any, error) {
			return work(&managed{ManagedTransaction: tx, s: s, ctx: ctx})
		}
		var e error
		if write {
			out, e = s.Session.ExecuteWrite(ctx, fn, opts...)
		} else {
			out, e = s.Session.ExecuteRead(ctx, fn, opts...)
		}
		s.finishPending(e)
		return e
	})
	return
}
func (s *Session) ExecuteRead(ctx context.Context, work native.ManagedTransactionWork, opts ...func(*native.TransactionConfig)) (any, error) {
	return s.managed(ctx, false, work, opts...)
}
func (s *Session) ExecuteWrite(ctx context.Context, work native.ManagedTransactionWork, opts ...func(*native.TransactionConfig)) (any, error) {
	return s.managed(ctx, true, work, opts...)
}

type managed struct {
	native.ManagedTransaction
	s   *Session
	ctx context.Context
}

func (m *managed) Run(ctx context.Context, q string, params map[string]any) (native.Result, error) {
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(m.ctx))
	return m.s.query(ctx, q, func(ctx context.Context) (native.Result, error) { return m.ManagedTransaction.Run(ctx, q, params) })
}

type transaction struct {
	native.ExplicitTransaction
	s   *Session
	ctx context.Context
	end func(error)
}

func (t *transaction) Run(ctx context.Context, q string, params map[string]any) (native.Result, error) {
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(t.ctx))
	return t.s.query(ctx, q, func(ctx context.Context) (native.Result, error) { return t.ExplicitTransaction.Run(ctx, q, params) })
}
func (t *transaction) Commit(ctx context.Context) error {
	err := t.ExplicitTransaction.Commit(ctx)
	t.s.finishPending(err)
	t.end(err)
	return err
}
func (t *transaction) Rollback(ctx context.Context) error {
	err := t.ExplicitTransaction.Rollback(ctx)
	t.s.finishPending(err)
	t.end(err)
	return err
}
func (t *transaction) Close(ctx context.Context) error {
	err := t.ExplicitTransaction.Close(ctx)
	t.s.finishPending(err)
	t.end(err)
	return err
}

type result struct {
	native.Result
	end func(error)
}

func (r *result) Next(ctx context.Context) bool {
	ok := r.Result.Next(ctx)
	if !ok {
		r.end(r.Result.Err())
	}
	return ok
}
func (r *result) NextRecord(ctx context.Context, record **native.Record) bool {
	ok := r.Result.NextRecord(ctx, record)
	if !ok {
		r.end(r.Result.Err())
	}
	return ok
}
func (r *result) Collect(ctx context.Context) ([]*native.Record, error) {
	v, err := r.Result.Collect(ctx)
	r.end(err)
	return v, err
}
func (r *result) Single(ctx context.Context) (*native.Record, error) {
	v, err := r.Result.Single(ctx)
	r.end(err)
	return v, err
}
func (r *result) Consume(ctx context.Context) (native.ResultSummary, error) {
	v, err := r.Result.Consume(ctx)
	r.end(err)
	return v, err
}
func (r *result) Records(ctx context.Context) func(func(*native.Record, error) bool) {
	return func(yield func(*native.Record, error) bool) {
		r.Result.Records(ctx)(func(v *native.Record, err error) bool {
			if err != nil {
				r.end(err)
			}
			return yield(v, err)
		})
		r.end(r.Result.Err())
	}
}

var _ native.Session = (*Session)(nil)
var _ native.Result = (*result)(nil)

func (r *result) Err() error {
	err := r.Result.Err()
	if err != nil {
		r.end(err)
	}
	return err
}
func (r *result) Peek(ctx context.Context) bool {
	ok := r.Result.Peek(ctx)
	if !ok {
		r.end(r.Result.Err())
	}
	return ok
}
func (r *result) PeekRecord(ctx context.Context, record **native.Record) bool {
	ok := r.Result.PeekRecord(ctx, record)
	if !ok {
		r.end(r.Result.Err())
	}
	return ok
}
