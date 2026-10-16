package gorm

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type testDialect struct{ db *sql.DB }

func (testDialect) Name() string { return "postgres" }
func (d testDialect) Initialize(db *native.DB) error {
	db.ConnPool = d.db
	callbacks.RegisterDefaultCallbacks(db, &callbacks.Config{})
	return nil
}
func (testDialect) Migrator(*native.DB) native.Migrator { return nil }
func (testDialect) DataTypeOf(*schema.Field) string     { return "text" }
func (testDialect) DefaultValueOf(*schema.Field) clause.Expression {
	return clause.Expr{SQL: "DEFAULT"}
}
func (testDialect) BindVarTo(w clause.Writer, _ *native.Statement, _ any) { _ = w.WriteByte('?') }
func (testDialect) QuoteTo(w clause.Writer, s string)                     { _, _ = w.WriteString(s) }
func (testDialect) Explain(query string, _ ...any) string                 { return query }
func TestCallbacksAndPrivacy(t *testing.T) {
	p := testkit.New(t)
	name, driver := testkit.RegisterSQL(t)
	sqlDB, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := native.Open(testDialect{sqlDB}, &native.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := Instrument(db, p, WithDBName("payment")); err != nil {
		t.Fatal(err)
	}
	ctx, parent := p.TP.Tracer("application").Start(context.Background(), "request")
	type Payment struct{ Value string }
	if err := db.WithContext(ctx).Create(&Payment{Value: "SECRET"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&Payment{}).Where("value = ?", "SECRET").Update("value", "SECRET").Error; err != nil {
		t.Fatal(err)
	}
	driver.ExecError = errors.New("SECRET")
	if db.WithContext(ctx).Exec("DELETE SECRET").Error == nil {
		t.Fatal("expected error")
	}
	parent.End()
	if len(p.Recorder.Ended()) < 4 {
		t.Fatal("missing ORM spans")
	}
	p.AssertPrivate(t, "SECRET")
	if len(p.Collect(t).ScopeMetrics) == 0 {
		t.Fatal("missing metrics")
	}
	if Instrument(db, p) == nil {
		t.Fatal("duplicate instrumentation accepted")
	}
}
