package goslogx

import (
	"testing"

	resourceimpl "github.com/muhammadluth/goslogx/internal/telemetry/resource"

	"go.opentelemetry.io/otel/attribute"
)

func completeResourceConfig() *Config {
	config := defaultConfig()
	config.ServiceName = "payment-service"
	config.Tracing = TracingConfig{
		Enabled:               true,
		Endpoint:              "otel-collector:4317",
		ServiceVersion:        "1.4.2",
		DeploymentEnvironment: "production",
		ServiceInstanceID:     "payment-api-7c9f",
		ResourceAttributes: map[string]string{
			"team.name": "payments",
		},
	}
	return config
}

func resourceAttribute(t testing.TB, config *Config, key attribute.Key) string {
	t.Helper()

	resource, err := resourceimpl.New(config.ServiceName, resourceimpl.Resolve(resourceimpl.Config(config.Resource), resourceimpl.Config{ServiceVersion: config.Tracing.ServiceVersion, DeploymentEnvironment: config.Tracing.DeploymentEnvironment, ServiceInstanceID: config.Tracing.ServiceInstanceID, ResourceAttributes: config.Tracing.ResourceAttributes}))
	if err != nil {
		t.Fatalf("newServiceResource() error = %v", err)
	}
	value, ok := resource.Set().Value(key)
	if !ok {
		t.Fatalf("resource attribute %q is missing", key)
	}
	return value.AsString()
}
