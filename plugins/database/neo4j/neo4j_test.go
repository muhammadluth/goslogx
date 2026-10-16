package neo4j

import (
	"context"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"testing"
)

type fakeSession struct {
	native.Session
	err error
	tx  *fakeTx
}

func (f *fakeSession) Run(context.Context, string, map[string]any, ...func(*native.TransactionConfig)) (native.Result, error) {
	return &fakeResult{}, f.err
}
func (f *fakeSession) Close(context.Context) error { return nil }
func (f *fakeSession) BeginTransaction(context.Context, ...func(*native.TransactionConfig)) (native.ExplicitTransaction, error) {
	return f.tx, nil
}
func (f *fakeSession) ExecuteWrite(ctx context.Context, work native.ManagedTransactionWork, _ ...func(*native.TransactionConfig)) (any, error) {
	return work(f.tx)
}

type fakeTx struct{ native.ExplicitTransaction }

func (*fakeTx) Run(context.Context, string, map[string]any) (native.Result, error) {
	return &fakeResult{}, nil
}
func (*fakeTx) Commit(context.Context) error { return nil }

type fakeResult struct{ native.Result }

func (*fakeResult) Next(context.Context) bool { return false }
func (*fakeResult) Err() error                { return nil }
func TestSessionAndTransaction(t *testing.T) {
	p := testkit.New(t)
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	f := &fakeSession{tx: &fakeTx{}}
	s, err := WrapSession(f, p, WithDBName("orders"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, "MATCH SECRET", map[string]any{"secret": "SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	r.Next(ctx)
	if len(s.pending) != 0 {
		t.Fatal("retained finished cursor")
	}
	tx, err := s.BeginTransaction(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, err = tx.Run(ctx, "MATCH SECRET", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Next(ctx)
	tx.Commit(ctx)
	_, err = s.ExecuteWrite(ctx, func(tx native.ManagedTransaction) (any, error) {
		r, e := tx.Run(ctx, "MATCH SECRET", nil)
		if e == nil {
			r.Next(ctx)
		}
		return nil, e
	})
	if err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("SECRET")
	if _, err = s.Run(ctx, "MATCH SECRET", nil); err != f.err {
		t.Fatal("error identity")
	}
	s.Close(ctx)
	parent.End()
	if len(p.Recorder.Ended()) != 8 {
		t.Fatal(len(p.Recorder.Ended()))
	}
	p.AssertPrivate(t, "SECRET")
}
