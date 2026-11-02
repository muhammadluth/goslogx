package telemetry

// ApplyOptions preserves the shared Option contract while allowing an adapter
// to store its own upstream-specific settings without a shared giant config.
func ApplyOptions(c *Config, target any, options ...Option) {
	previous := c.optionTarget
	c.optionTarget = target
	defer func() { c.optionTarget = previous }()
	for _, option := range options {
		if option != nil {
			option(c)
		}
	}
}

// PluginOption applies only to its owning adapter. Common options still apply
// directly to Config and remain assignable between historical plugin aliases.
func PluginOption[T any](apply func(*T)) Option {
	return func(c *Config) {
		if target, ok := c.optionTarget.(*T); ok {
			apply(target)
		}
	}
}
