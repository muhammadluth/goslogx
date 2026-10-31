package gorm

import (
	"context"
	"database/sql"
	"errors"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	native "gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
	"strings"
	"testing"
)

func TestDirectNativeCallbacksParity(t *testing.T) {
	for _, failure := range []error{nil, errors.New("SECRET")} {
		var spans [2][]sdktrace.ReadOnlySpan
		for index, adapter := range []bool{false, true} {
			p := testkit.New(t)
			name, d := testkit.RegisterSQL(t)
			raw, err := sql.Open(name, "")
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			db, err := native.Open(testDialect{raw}, &native.Config{Logger: logger.Discard})
			if err != nil {
				t.Fatal(err)
			}
			if adapter {
				err = Instrument(db, p)
			} else {
				err = db.Use(tracing.NewPlugin(tracing.WithTracerProvider(p.TP), tracing.WithoutMetrics(), tracing.WithoutQueryVariables()))
			}
			if err != nil {
				t.Fatal(err)
			}
			sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
			ctx := trace.ContextWithSpanContext(context.Background(), sc)
			d.ExecError = failure
			if err = db.WithContext(ctx).Exec("UPDATE orders SET value=?", "SECRET").Error; err != failure {
				t.Fatal("native error identity", err)
			}
			spans[index] = p.Recorder.Ended()
			if adapter {
				p.AssertPrivate(t, "SECRET")
			}
		}
		if len(spans[0]) != 1 || len(spans[1]) != 1 {
			t.Fatal("duplicate callbacks", spans)
		}
		want, got := spans[0][0], spans[1][0]
		if got.Name() != want.Name() || got.SpanKind() != want.SpanKind() || !got.Parent().Equal(want.Parent()) || got.Status().Code != want.Status().Code || len(got.Events()) != len(want.Events()) {
			t.Fatal("native callback semantics", got.Name(), want.Name())
		}
	}
}

func TestDiagnosticOptionsPreserveSanitization(t *testing.T) {
	p := testkit.New(t)
	name, d := testkit.RegisterSQL(t)
	raw, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db, err := native.Open(testDialect{raw}, &native.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err = Instrument(db, p, WithRecordStackTrace(true), WithServerAddress(false), WithQueryVariables(true), WithStatementCapture(true), WithStatementSanitizer(func(query string) string { calls++; return "UPDATE orders SET value=?" })); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("SECRET")
	d.ExecError = failure
	if err = db.Exec("UPDATE orders SET value=?", "SECRET").Error; err != failure {
		t.Fatal("error identity")
	}
	spans := p.Recorder.Ended()
	if calls != 1 || len(spans) != 1 || len(spans[0].Events()) != 1 {
		t.Fatal("sanitizer or exception duplication", calls, spans)
	}
	stack := false
	for _, a := range spans[0].Events()[0].Attributes {
		if a.Key == "exception.stacktrace" && a.Value.AsString() != "" {
			stack = true
		}
	}
	if stack {
		t.Fatal("upstream does not attach stacks to ordinary query errors")
	}
	p.AssertPrivate(t, "SECRET")
}

func TestNativeEndPreservesPanicIdentityAndPrivateStack(t *testing.T) {
	for _, stack := range []bool{false, true} {
		p := testkit.New(t)
		name, _ := testkit.RegisterSQL(t)
		raw, _ := sql.Open(name, "")
		defer raw.Close()
		db, err := native.Open(testDialect{raw}, &native.Config{Logger: logger.Discard})
		if err != nil {
			t.Fatal(err)
		}
		failure := errors.New("SECRET")
		if err = Instrument(db, p, WithRecordStackTrace(stack), WithStatementCapture(true), WithStatementSanitizer(func(string) string { panic(failure) })); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recover() != failure {
					t.Error("panic identity")
				}
			}()
			db.Exec("SELECT 1")
			t.Error("panic was swallowed")
		}()
		spans := p.Recorder.Ended()
		if len(spans) != 1 || len(spans[0].Events()) != 1 {
			t.Fatal("missing panic telemetry", spans)
		}
		captured := false
		for _, a := range spans[0].Events()[0].Attributes {
			if a.Key == "exception.stacktrace" {
				captured = a.Value.AsString() != ""
			}
		}
		if captured != stack {
			t.Fatal("native stack option was not preserved")
		}
		p.AssertPrivate(t, "SECRET")
	}
}

func TestExplicitRawStatementCapture(t *testing.T) {
	p := testkit.New(t)
	name, _ := testkit.RegisterSQL(t)
	raw, _ := sql.Open(name, "")
	defer raw.Close()
	db, err := native.Open(testDialect{raw}, &native.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err = Instrument(db, p, WithRawStatement(true)); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("UPDATE orders SET value='diagnostic-value'").Error; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range p.Recorder.Ended() {
		for _, a := range s.Attributes() {
			if a.Key == "db.query.text" && strings.Contains(a.Value.AsString(), "diagnostic-value") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("explicit raw capture missing")
	}
}
