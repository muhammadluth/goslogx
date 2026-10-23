# RESOURCE IDENTITY

OpenTelemetry resources identify the process that produced traces and metrics. Configure one shared resource for managed providers:

```go
goslogx.WithResource(goslogx.ResourceConfig{
    ServiceVersion:        "1.2.0",
    DeploymentEnvironment: "production",
    ServiceInstanceID:     "payment-api-01",
})
```

`WithServiceName("payment-api")` supplies `service.name`. `ResourceConfig` supplies `service.version`, `service.instance.id`, and `deployment.environment.name`. `ResourceAttributes` may add bounded deployment metadata, but reserved service fields cannot be overridden through that map.

Tempo and other trace backends group traces by service resource. Mimir/Prometheus-compatible systems use resource conversion to identify metric producers. Grafana uses the same identity to navigate between signals. Keep `service.name` stable across replicas; put the replica identifier in `service.instance.id`.

Application-owned providers must be created with their own OpenTelemetry resource because `WithResource` configures goslogx-owned providers only. See [provider ownership](../architecture/provider-ownership.md).
