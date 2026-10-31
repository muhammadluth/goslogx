package neo4j

import (
	"context"
	"errors"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"go.opentelemetry.io/otel/codes"
)

type lifecycleResult struct {
	native.Result
	remaining int
	err       error
}

func (r *lifecycleResult) Err() error { return r.err }
func (r *lifecycleResult) Next(context.Context) bool {
	r.remaining--
	return r.remaining >= 0 && r.err == nil
}
func (r *lifecycleResult) NextRecord(ctx context.Context, out **native.Record) bool {
	*out = &native.Record{}
	return r.Next(ctx)
}
func (r *lifecycleResult) Peek(context.Context) bool { return r.remaining > 0 && r.err == nil }
func (r *lifecycleResult) PeekRecord(ctx context.Context, out **native.Record) bool {
	*out = &native.Record{}
	return r.Peek(ctx)
}
func (r *lifecycleResult) Collect(context.Context) ([]*native.Record, error) {
	return []*native.Record{{}}, r.err
}
func (r *lifecycleResult) Single(context.Context) (*native.Record, error) {
	return &native.Record{}, r.err
}
func (r *lifecycleResult) Consume(context.Context) (native.ResultSummary, error) { return nil, r.err }
func (r *lifecycleResult) Records(ctx context.Context) func(func(*native.Record, error) bool) {
	return func(yield func(*native.Record, error) bool) {
		for r.Next(ctx) {
			if !yield(&native.Record{}, nil) {
				return
			}
		}
		if r.err != nil {
			yield(nil, r.err)
		}
	}
}

type lifecycleSession struct {
	native.Session
	result   *lifecycleResult
	tx       *lifecycleTx
	err      error
	attempts int
}

func (s *lifecycleSession) Run(context.Context, string, map[string]any, ...func(*native.TransactionConfig)) (native.Result, error) {
	return s.result, s.err
}
func (s *lifecycleSession) Close(context.Context) error { return s.err }
func (s *lifecycleSession) BeginTransaction(context.Context, ...func(*native.TransactionConfig)) (native.ExplicitTransaction, error) {
	return s.tx, s.err
}
func (s *lifecycleSession) ExecuteWrite(ctx context.Context, w native.ManagedTransactionWork, opts ...func(*native.TransactionConfig)) (any, error) {
	return s.ExecuteRead(ctx, w, opts...)
}
func (s *lifecycleSession) ExecuteRead(_ context.Context, w native.ManagedTransactionWork, _ ...func(*native.TransactionConfig)) (any, error) {
	var value any
	var err error
	for range s.attempts {
		value, err = w(s.tx)
	}
	return value, err
}

type lifecycleTx struct {
	native.ExplicitTransaction
	result *lifecycleResult
	err    error
}

func (tx *lifecycleTx) Run(context.Context, string, map[string]any) (native.Result, error) {
	return tx.result, tx.err
}
func (tx *lifecycleTx) Commit(context.Context) error   { return tx.err }
func (tx *lifecycleTx) Rollback(context.Context) error { return tx.err }
func (tx *lifecycleTx) Close(context.Context) error    { return tx.err }

func TestResultCompletionAndEarlyIteratorExit(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("SECRET")
	for _, mode := range []string{"next", "record", "collect", "single", "consume", "peek", "peekrecord", "records", "break", "error"} {
		for _, queryErr := range []error{nil, failure} {
			t.Run(mode+"/"+string(rune('0'+boolInt(queryErr != nil))), func(t *testing.T) {
				p := testkit.New(t)
				f := &lifecycleSession{result: &lifecycleResult{remaining: 1}}
				s, err := WrapSession(f, p, WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "MATCH (?)" }))
				if err != nil {
					t.Fatal(err)
				}
				r, err := s.Run(ctx, "MATCH SECRET", nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Recorder.Ended()) != 0 {
					t.Fatal("query ended before reading")
				}
				var record *native.Record
				if !r.Peek(ctx) || !r.PeekRecord(ctx, &record) || r.Err() != nil {
					t.Fatal("result methods changed")
				}
				f.result.err = queryErr
				switch mode {
				case "next":
					for r.Next(ctx) {
					}
				case "record":
					for r.NextRecord(ctx, &record) {
					}
				case "collect":
					_, err = r.Collect(ctx)
				case "single":
					_, err = r.Single(ctx)
				case "consume":
					_, err = r.Consume(ctx)
				case "peek":
					f.result.remaining = 0
					r.Peek(ctx)
				case "peekrecord":
					f.result.remaining = 0
					r.PeekRecord(ctx, &record)
				case "records":
					for range r.Records(ctx) {
					}
				case "break":
					for range r.Records(ctx) {
						break
					}
					if queryErr == nil && len(p.Recorder.Ended()) != 0 {
						t.Fatal("iterator break closed live cursor")
					}
					_, err = r.Consume(ctx)
				case "error":
					err = r.Err()
					if queryErr == nil {
						_, err = r.Consume(ctx)
					}
				}
				if err != nil && err != queryErr {
					t.Fatal("result error identity")
				}
				spans := p.Recorder.Ended()
				if len(spans) != 1 || spans[0].Name() != "MATCH" || len(s.pending) != 0 {
					t.Fatal("result lifetime")
				}
				if (spans[0].Status().Code == codes.Error) != (queryErr != nil) {
					t.Fatal("result status")
				}
				p.AssertPrivate(t, "SECRET")
			})
		}
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestTransactionCompletionRetriesAndPanics(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("SECRET")
	for _, completion := range []string{"commit", "rollback", "close", "read", "write", "panic"} {
		t.Run(completion, func(t *testing.T) {
			p := testkit.New(t)
			f := &lifecycleSession{tx: &lifecycleTx{result: &lifecycleResult{remaining: 1}}, attempts: 2}
			s, err := WrapSession(f, p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = WrapSession(s, p); err == nil {
				t.Fatal("duplicate accepted")
			}
			if completion == "read" || completion == "write" || completion == "panic" {
				calls := 0
				work := func(tx native.ManagedTransaction) (any, error) {
					if len(s.pending) != 0 {
						t.Fatal("previous retry retained query")
					}
					calls++
					_, err := tx.Run(ctx, "RETURN SECRET", nil)
					if err != nil {
						t.Fatal(err)
					}
					if completion == "panic" {
						panic(failure)
					}
					return 42, failure
				}
				if completion == "panic" {
					func() {
						defer func() {
							if recover() != failure {
								t.Fatal("panic identity")
							}
						}()
						_, _ = s.ExecuteWrite(ctx, work)
					}()
				} else {
					var value any
					if completion == "read" {
						value, err = s.ExecuteRead(ctx, work)
					} else {
						value, err = s.ExecuteWrite(ctx, work)
					}
					if value != 42 || err != failure || calls != 2 {
						t.Fatal("managed retry behavior changed")
					}
				}
			} else {
				tx, err := s.BeginTransaction(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tx.Run(ctx, "CREATE SECRET", nil); err != nil {
					t.Fatal(err)
				}
				f.tx.err = failure
				switch completion {
				case "commit":
					err = tx.Commit(ctx)
				case "rollback":
					err = tx.Rollback(ctx)
				case "close":
					err = tx.Close(ctx)
				}
				if err != failure {
					t.Fatal("transaction error identity")
				}
			}
			if len(s.pending) != 0 {
				t.Fatal("transaction retained results")
			}
			for _, span := range p.Recorder.Ended() {
				if span.Status().Code != codes.Error {
					t.Fatal("transaction error lost")
				}
			}
			p.AssertPrivate(t, "SECRET")
		})
	}
	p := testkit.New(t)
	f := &lifecycleSession{result: &lifecycleResult{remaining: 1}}
	s, _ := WrapSession(f, p)
	_, _ = s.Run(ctx, "SECRET", nil)
	f.err = failure
	if _, err := s.BeginTransaction(ctx); err != failure {
		t.Fatal("begin failure")
	}
	if s.Close(ctx) != failure || len(s.pending) != 0 {
		t.Fatal("session completion")
	}
	if _, err := WrapSession(nil, nil); err == nil {
		t.Fatal("nil accepted")
	}
	if _, err := WrapSession(f, nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe capture accepted")
	}
}
