package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"strings"
	"testing"
	"time"
)

func TestDatabaseBoundaryPreservesSemanticsAndSanitizesStatements(t *testing.T) {
	p := testkit.New(t)
	cfg := Config{StatementCapture: true, StatementSanitizer: func(string) string { return "SELECT ?" }}
	WithTraceAttributes(attribute.String("app.component", "store"))(&cfg)
	tp := ConfiguredDBProvider(p.TP, "adapter", cfg)
	link := trace.Link{SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}}), Attributes: []attribute.KeyValue{attribute.String("app.phase", "retry"), attribute.String("db.statement", "SECRET")}}
	start := time.Now().Add(-time.Second)
	ctx, span := tp.Tracer("native", trace.WithInstrumentationVersion("1.2.3")).Start(context.Background(), "FindOne", trace.WithSpanKind(trace.SpanKindClient), trace.WithNewRoot(), trace.WithTimestamp(start), trace.WithLinks(link), trace.WithAttributes(attribute.String("db.system", "mongodb"), attribute.String("db.query.text", "SECRET"), attribute.String("network.peer.address", "db.internal"), attribute.Int("network.peer.port", 27017)))
	if span.TracerProvider() == nil || !trace.SpanContextFromContext(ctx).IsValid() {
		t.Fatal("span context")
	}
	span.SetName("find")
	span.SetAttributes(attribute.String("db.statement", "SECRET"), attribute.String("db.connection_string", "postgres://user:SECRET@db.internal:5432/db?token=SECRET"), attribute.Int("pgx.rows_affected", 1))
	span.AddLink(link)
	span.AddEvent("sql.rows.next", trace.WithAttributes(attribute.String("db.statement", "SECRET")))
	span.RecordError(errors.New("SECRET"))
	span.SetStatus(codes.Error, "SECRET")
	span.End()
	got := p.Recorder.Ended()[0]
	if got.Name() != "find" || got.SpanKind() != trace.SpanKindClient || !got.StartTime().Equal(start) || len(got.Links()) != 2 || len(got.Events()) != 2 || got.InstrumentationScope().Version != "1.2.3" {
		t.Fatalf("lost semantics: %v", got)
	}
	if got.Events()[0].Name != "sql.rows.next" || len(got.Links()[0].Attributes) != 1 {
		t.Fatal("event/link metadata")
	}
	p.AssertPrivate(t, "SECRET")
	if !strings.Contains(fmt.Sprint(got.Attributes()), "SELECT ?") {
		t.Fatal("sanitizer not applied")
	}
	_, plain := SafeDBProvider(p.TP, "legacy").Tracer("native").Start(ctx, "SELECT", trace.WithAttributes(attribute.String("db.statement", "SECRET")))
	plain.End()
	p.AssertPrivate(t, "SECRET")
}

func TestSQLPoolMeasuresPublicStats(t *testing.T) {
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(7)
	if err = SQLPool(p.MP.Meter("pool"), db, "appdb"); err != nil {
		t.Fatal(err)
	}
	if err = db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprint(p.Collect(t))
	for _, want := range []string{"db.client.connection.count", "db.client.connection.max", "idle", "used", "appdb"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
}
