package gorm

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	native "gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

type namedDialect struct {
	testDialect
	name string
}

func (d namedDialect) Name() string { return d.name }

type recordDTO struct {
	Value int64 `json:"value"`
}

func TestAllCallbacksTransactionsAndSignals(t *testing.T) {
	for n, name := range []string{"mysql", "sqlite", "sqlserver", "other"} {
		t.Run(name, func(t *testing.T) {
			p := testkit.New(t)
			driverName, d := testkit.RegisterSQL(t)
			raw, _ := sql.Open(driverName, "")
			defer raw.Close()
			db, err := native.Open(namedDialect{testDialect{raw}, name}, &native.Config{Logger: logger.Discard})
			if err != nil {
				t.Fatal(err)
			}
			tracingEnabled, metricsEnabled := n != 1 && n != 3, n != 2 && n != 3
			err = Instrument(db, p, WithTracing(tracingEnabled), WithMetrics(metricsEnabled), WithStatementCapture(true), WithStatementSanitizer(func(string) string { return "SELECT ?" }))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			var result recordDTO
			if err = db.WithContext(ctx).Take(&result).Error; err != nil || result.Value != 1 {
				t.Fatal("query", err, result)
			}
			if err = db.WithContext(ctx).Where("value=?", 1).Delete(&recordDTO{}).Error; err != nil {
				t.Fatal("delete", err)
			}
			var value int
			if err = db.WithContext(ctx).Raw("SELECT ?", "SECRET").Row().Scan(&value); err != nil || value != 1 {
				t.Fatal("row", err)
			}
			d.QueryError = native.ErrRecordNotFound
			if err = db.WithContext(ctx).Take(&result).Error; !errors.Is(err, native.ErrRecordNotFound) {
				t.Fatal("not found identity", err)
			}
			d.QueryError = nil
			if (len(p.Recorder.Ended()) > 0) != tracingEnabled || (len(p.Collect(t).ScopeMetrics) > 0) != metricsEnabled {
				t.Fatal("independent signals")
			}
			p.AssertPrivate(t, "SECRET")
			for _, failure := range []error{nil, errors.New("SECRET transaction")} {
				err = Transaction(ctx, db, p, func(tx *native.DB) error {
					if tx.Statement.Context == nil {
						t.Fatal("lost context")
					}
					return failure
				})
				if err != failure {
					t.Fatal("transaction identity", err)
				}
			}
		})
	}
}

func TestValidationAndNativePluginCoexistence(t *testing.T) {
	if Instrument(nil, nil) == nil || Instrument(&native.DB{}, nil) == nil || Transaction(context.Background(), nil, nil, nil) == nil {
		t.Fatal("nil accepted")
	}
	name, _ := testkit.RegisterSQL(t)
	raw, _ := sql.Open(name, "")
	defer raw.Close()
	db, err := native.Open(testDialect{raw}, &native.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err = Instrument(db, nil, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe capture")
	}
	if err = Transaction(context.Background(), db, nil, func(*native.DB) error { return nil }, WithStatementCapture(true)); err == nil {
		t.Fatal("unsafe transaction capture")
	}
	p := testkit.New(t)
	if err = db.Use(tracing.NewPlugin(tracing.WithTracerProvider(p.TP), tracing.WithoutMetrics())); err != nil {
		t.Fatal(err)
	}
	if err = Instrument(db, p); err == nil {
		t.Fatal("native duplicate accepted")
	}
}
