package telemetry

import "testing"

type adapterOptionsFixture struct {
	Config
	value string
}

func TestPluginOptionsPreserveCommonContractAndOrder(t *testing.T) {
	var local adapterOptionsFixture
	set := func(value string) Option { return PluginOption(func(c *adapterOptionsFixture) { c.value = value }) }
	ApplyOptions(&local.Config, &local, WithTracing(false), set("first"), nil, WithMetrics(false), set("last"))
	if !local.TracingDisabled || !local.MetricsDisabled || local.value != "last" || local.optionTarget != nil {
		t.Fatalf("options: %+v", local)
	}
	var other Config
	ApplyOptions(&other, &other, set("ignored"), WithTracing(false))
	if !other.TracingDisabled || other.optionTarget != nil {
		t.Fatal("foreign option changed common behavior")
	}
}
