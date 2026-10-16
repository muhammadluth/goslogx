// Package dynamodb installs official AWS SDK v2 OpenTelemetry middleware.
package dynamodb

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
	"github.com/muhammadluth/goslogx"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	"go.opentelemetry.io/otel/attribute"
	"time"
)

const ScopeName = "github.com/muhammadluth/goslogx/plugins/database/dynamodb"

type Option = telemetry.Option

var WithDBName = telemetry.WithDBName
var WithTracing = telemetry.WithTracing
var WithMetrics = telemetry.WithMetrics

// Instrument applies to clients subsequently constructed from cfg. Use a config
// dedicated to DynamoDB. SDK signing, credential resolution and retries remain native.
func Instrument(cfg *aws.Config, p goslogx.TelemetryProvider, opts ...Option) error {
	if cfg == nil {
		return errors.New("dynamodb: nil AWS config")
	}
	t, err := telemetry.New(p, ScopeName, "aws.dynamodb", false, opts...)
	if err != nil {
		return err
	}
	tp, _, prop := telemetry.Resolve(p, t.Config)
	var nativeOptions []func(*middleware.Stack) error
	otelaws.AppendMiddlewares(&nativeOptions, otelaws.WithTracerProvider(telemetry.SafeDBProvider(tp, ScopeName)), otelaws.WithTextMapPropagator(prop), otelaws.WithAttributeBuilder(func(ctx context.Context, _ middleware.InitializeInput, _ middleware.InitializeOutput) []attribute.KeyValue {
		a := []attribute.KeyValue{attribute.String("db.system.name", "aws.dynamodb"), attribute.String("db.operation.name", telemetry.DBOperation(awsmiddleware.GetOperationName(ctx)))}
		if t.Config.Namespace != "" {
			a = append(a, attribute.String("db.namespace", t.Config.Namespace))
		}
		return a
	}))
	cfg.APIOptions = append(cfg.APIOptions, func(stack *middleware.Stack) error {
		if _, ok := stack.Initialize.Get(ScopeName); ok {
			return errors.New("dynamodb: already instrumented")
		}
		if _, ok := stack.Initialize.Get("OTelInitializeMiddlewareBefore"); ok {
			return errors.New("dynamodb: OpenTelemetry middleware already installed")
		}
		for _, install := range nativeOptions {
			if err := install(stack); err != nil {
				return err
			}
		}
		return stack.Initialize.Add(middleware.InitializeMiddlewareFunc(ScopeName, func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
			start := time.Now()
			out, meta, err := next.HandleInitialize(ctx, in)
			t.RecordDatabase(ctx, awsmiddleware.GetOperationName(ctx), time.Since(start), err)
			return out, meta, err
		}), middleware.Before)
	})
	return nil
}
