package resourceimpl

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

func TestServiceResource(t *testing.T) {
	testServiceName = "payment-service"
	config := completeResourceConfig()
	tests := []struct {
		name string
		key  attribute.Key
		want string
	}{{name: "service name", key: semconv.ServiceNameKey, want: "payment-service"}, {name: "service version", key: semconv.ServiceVersionKey, want: "1.4.2"},
		{name: "service instance ID", key: semconv.ServiceInstanceIDKey, want: "payment-api-7c9f"},
		{name: "deployment environment", key: semconv.DeploymentEnvironmentNameKey, want: "production"},
		{name: "custom attribute", key: attribute.Key("team.name"), want: "payments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceAttribute(t, config, tt.key); got != tt.want {
				t.Errorf("resource attribute %q = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	t.Run("default SDK resource remains available", func(t *testing.T) {
		if got := resourceAttribute(t, config, semconv.TelemetrySDKNameKey); got == "" {
			t.Error("telemetry.sdk.name is empty")
		}
	})
}
func TestResourceConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing service name", mutate: func(c *Config) { testServiceName = "" }},
		{name: "unknown service name", mutate: func(c *Config) { testServiceName = "unknown" }},
		{name: "missing service version", mutate: func(c *Config) { c.ServiceVersion = "" }},
		{name: "missing environment", mutate: func(c *Config) { c.DeploymentEnvironment = "" }},
		{name: "missing instance ID", mutate: func(c *Config) { c.ServiceInstanceID = "" }},
		{name: "empty custom key", mutate: func(c *Config) { c.ResourceAttributes[""] = "value" }},
		{name: "reserved service name", mutate: func(c *Config) { c.ResourceAttributes["service.name"] = "other" }},
		{name: "reserved service version", mutate: func(c *Config) { c.ResourceAttributes["service.version"] = "other" }},
		{name: "reserved instance ID", mutate: func(c *Config) { c.ResourceAttributes["service.instance.id"] = "other" }},
		{name: "reserved environment", mutate: func(c *Config) { c.ResourceAttributes["deployment.environment.name"] = "other" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServiceName = "payment-service"
			config := completeResourceConfig()
			tt.mutate(config)
			if err := Validate(testServiceName, *config); err == nil {
				t.Fatal("validateResourceConfig() error = nil")
			}
		})
	}
}

var testServiceName = "payment-service"

func completeResourceConfig() *Config {
	return &Config{ServiceVersion: "1.4.2", DeploymentEnvironment: "production", ServiceInstanceID: "payment-api-7c9f", ResourceAttributes: map[string]string{"team.name": "payments"}}
}
func resourceAttribute(t testing.TB, cfg *Config, key attribute.Key) string {
	t.Helper()
	r, err := New(testServiceName, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := r.Set().Value(key)
	if !ok {
		t.Fatalf("missing resource attribute %s", key)
	}
	return value.AsString()
}
