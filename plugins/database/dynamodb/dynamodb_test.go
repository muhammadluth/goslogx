package dynamodb

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	awm "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/smithy-go/middleware"
	"github.com/muhammadluth/goslogx/internal/integration/telemetry/testkit"
	"testing"
)

func TestMiddlewarePrivacyAndDuplicate(t *testing.T) {
	p := testkit.New(t)
	cfg := aws.Config{}
	if err := Instrument(&cfg, p); err != nil {
		t.Fatal(err)
	}
	stack := middleware.NewStack("GetItem", nil)
	for _, install := range cfg.APIOptions {
		if err := install(stack); err != nil {
			t.Fatal(err)
		}
	}
	if err := cfg.APIOptions[len(cfg.APIOptions)-1](stack); err == nil {
		t.Fatal("duplicate accepted")
	}
	ctx, parent := p.TP.Tracer("app").Start(context.Background(), "parent")
	ctx = awm.SetServiceID(ctx, "DynamoDB")
	ctx = awm.SetOperationName(ctx, "GetItem")
	sentinel := errors.New("SECRET")
	_, _, err := stack.Initialize.HandleMiddleware(ctx, &struct{ Item string }{"SECRET"}, middleware.HandlerFunc(func(context.Context, any) (any, middleware.Metadata, error) {
		return nil, middleware.Metadata{}, sentinel
	}))
	if err != sentinel {
		t.Fatal("error identity", err)
	}
	parent.End()
	if len(p.Recorder.Ended()) != 2 {
		t.Fatal(len(p.Recorder.Ended()))
	}
	p.AssertPrivate(t, "SECRET")
}
