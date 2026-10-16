package resourceimpl

import (
	"fmt"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// ResourceConfig supplies shared identity for managed traces and metrics.
// Nonempty fields take precedence over legacy TracingConfig identity fields.
type Config struct {
	ServiceVersion        string
	DeploymentEnvironment string
	ServiceInstanceID     string
	ResourceAttributes    map[string]string
}

var reservedResourceAttributeKeys = map[string]struct{}{
	string(semconv.ServiceNameKey):               {},
	string(semconv.ServiceVersionKey):            {},
	string(semconv.ServiceInstanceIDKey):         {},
	string(semconv.DeploymentEnvironmentNameKey): {},
}

func Resolve(primary, legacy Config) Config {
	if primary.ServiceVersion != "" {
		legacy.ServiceVersion = primary.ServiceVersion
	}
	if primary.DeploymentEnvironment != "" {
		legacy.DeploymentEnvironment = primary.DeploymentEnvironment
	}
	if primary.ServiceInstanceID != "" {
		legacy.ServiceInstanceID = primary.ServiceInstanceID
	}
	attrs := make(map[string]string, len(legacy.ResourceAttributes)+len(primary.ResourceAttributes))
	for k, v := range legacy.ResourceAttributes {
		attrs[k] = v
	}
	for k, v := range primary.ResourceAttributes {
		attrs[k] = v
	}
	legacy.ResourceAttributes = attrs
	return legacy
}
func Validate(serviceName string, cfg Config) error {
	if strings.TrimSpace(serviceName) == "" || serviceName == "unknown" {
		return fmt.Errorf("goslogx: service name is required when tracing is enabled")
	}
	if strings.TrimSpace(cfg.ServiceVersion) == "" {
		return fmt.Errorf("goslogx: tracing service version is required")
	}
	if strings.TrimSpace(cfg.DeploymentEnvironment) == "" {
		return fmt.Errorf("goslogx: tracing deployment environment is required")
	}
	if strings.TrimSpace(cfg.ServiceInstanceID) == "" {
		return fmt.Errorf("goslogx: tracing service instance ID is required")
	}
	for key := range cfg.ResourceAttributes {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("goslogx: tracing resource attributes contain an empty key")
		}
		if _, reserved := reservedResourceAttributeKeys[key]; reserved {
			return fmt.Errorf("goslogx: tracing resource attributes cannot override required service attributes")
		}
	}
	return nil
}
func New(serviceName string, cfg Config) (*resource.Resource, error) {
	if err := Validate(serviceName, cfg); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(cfg.ResourceAttributes))
	for key := range cfg.ResourceAttributes {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	attributes := make([]attribute.KeyValue, 0, len(keys)+4)
	for _, key := range keys {
		attributes = append(attributes, attribute.String(key, cfg.ResourceAttributes[key]))
	}
	attributes = append(attributes,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.ServiceInstanceID(cfg.ServiceInstanceID),
		semconv.DeploymentEnvironmentName(cfg.DeploymentEnvironment),
	)

	return resource.Merge(resource.Default(), resource.NewSchemaless(attributes...))
}
