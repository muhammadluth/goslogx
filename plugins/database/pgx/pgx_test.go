package pgx

import (
	"context"
	"errors"
	native "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
)

func TestHooksProvidersAndPrivacy(t *testing.T) {
	p := testkit.New(t)
	config, err := pgxpool.ParseConfig("postgres://user:SECRET@localhost/db")
	if err != nil {
		t.Fatal(err)
	}
	if err := InstrumentConfig(config, p, WithDBName("payment")); err != nil {
		t.Fatal(err)
	}
	if InstrumentConfig(config, p) == nil {
		t.Fatal("existing tracer overwritten")
	}
	hook := config.ConnConfig.Tracer.(*tracer)
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	query := hook.TraceQueryStart(ctx, nil, native.TraceQueryStartData{SQL: "SELECT 'SECRET'", Args: []any{"SECRET"}})
	hook.TraceQueryEnd(query, nil, native.TraceQueryEndData{Err: errors.New("SECRET")})
	batch := hook.TraceBatchStart(ctx, nil, native.TraceBatchStartData{})
	hook.TraceBatchQuery(batch, nil, native.TraceBatchQueryData{SQL: "UPDATE secret SET token='SECRET'"})
	hook.TraceBatchEnd(batch, nil, native.TraceBatchEndData{})
	copyCtx := hook.TraceCopyFromStart(ctx, nil, native.TraceCopyFromStartData{TableName: native.Identifier{"users"}})
	hook.TraceCopyFromEnd(copyCtx, nil, native.TraceCopyFromEndData{})
	parent.End()
	spans := p.Recorder.Ended()
	if len(spans) != 5 || spans[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatal("spans/parent")
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("missing metrics")
	}
}
