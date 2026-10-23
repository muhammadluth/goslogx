# TELEMETRY FLOW

The logger supplies one resource identity and explicit providers to every plugin. A server plugin extracts remote context and creates a server span. Database, cache, outbound HTTP, and messaging operations started with the derived context become children of that span. Context-aware logs read the active span identifiers.

Metrics share the provider resource but do not carry trace IDs as dimensions. Backends may attach exemplars when supported by the SDK and exporter.

```text
incoming carrier
    -> extract context
    -> server span
       -> application span
          -> database/cache/client/message span
          -> correlated goslogx log
    -> inject context into outgoing carrier
```

Resource attributes identify the service; span and metric attributes describe operations. See [propagation](../guides/propagation.md), [correlation](../guides/correlation.md), and [cardinality](../guides/cardinality.md).
