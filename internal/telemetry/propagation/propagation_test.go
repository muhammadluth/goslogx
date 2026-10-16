package propagationimpl

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestGenericCarriersAndPrivacyDefaults(t *testing.T) {
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	member, _ := baggage.NewMember("region", "test")
	bag, _ := baggage.New(member)
	ctx = baggage.ContextWithBaggage(ctx, bag)
	carrier := propagation.MapCarrier{}
	Inject(ctx, carrier)
	extracted := Extract(nil, carrier)
	if trace.SpanContextFromContext(extracted).TraceID() != sc.TraceID() || baggage.FromContext(extracted).Member("region").Value() != "test" {
		t.Fatal("legacy generic propagation")
	}
	safe := propagation.MapCarrier{}
	Default().Inject(ctx, safe)
	if safe.Get("baggage") != "" || safe.Get("traceparent") == "" {
		t.Fatal("privacy-safe propagation")
	}
}
