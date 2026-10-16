package goslogx

// ResourceConfig supplies shared identity for managed traces and metrics.
// Nonempty fields take precedence over legacy TracingConfig identity fields.
type ResourceConfig struct {
	ServiceVersion        string
	DeploymentEnvironment string
	ServiceInstanceID     string
	ResourceAttributes    map[string]string
}

// WithResource configures shared resource identity and snapshots the attribute map.
func WithResource(config ResourceConfig) Option {
	config.ResourceAttributes = cloneStringMap(config.ResourceAttributes)
	return func(c *Config) {
		copied := config
		copied.ResourceAttributes = cloneStringMap(config.ResourceAttributes)
		c.Resource = copied
	}
}
