# PRODUCTION OPERATIONS

Before production traffic:

- assign stable `service.name`, version, environment, and instance resource fields;
- use TLS and authenticated OTLP endpoints;
- choose a sampling ratio that fits traffic and retention budgets;
- keep statements, headers, bodies, payloads, caller discovery, and client IP capture disabled unless required;
- define bounded metric dimensions;
- monitor Collector health, exporter failures, retry queues, and backend ingestion;
- stop servers and consumers before telemetry shutdown.

goslogx does not collect host, container, CPU, memory, disk, or network bandwidth metrics. Use the Collector `hostmetrics` receiver, kubelet/cAdvisor, or platform monitoring for those signals. See [Collector operations](collector.md), [lifecycle](../guides/lifecycle.md), and [cardinality](../guides/cardinality.md).
