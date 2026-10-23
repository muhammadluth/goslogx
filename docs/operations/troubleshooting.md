# TROUBLESHOOTING

## TRACES ARE NOT VISIBLE

Check `logger.TracingError()`, verify the OTLP endpoint and TLS mode, confirm port `4317` is reachable, and inspect Collector exporter errors. Ensure the root sampling ratio is greater than zero and the trace backend pipeline is enabled.

## METRICS ARE NOT VISIBLE

Check `logger.MetricsError()`, wait at least one export interval, and verify that the Collector metrics pipeline exports to a metrics backend rather than Tempo. Pool metrics require the plugin-specific registration or observer described by that integration.

## LOGS ARE NOT VISIBLE

goslogx writes to its configured `io.Writer`; it does not export logs through OTLP. Verify stdout/file collection and the Alloy, Fluent Bit, or agent pipeline feeding Loki.

## CORRELATION FIELDS ARE MISSING

Use `InfoContext`, `DebugContext`, `WarningContext`, or `ErrorContext` with a context that contains a valid span. Ensure middleware passes the derived context into application code.

## DUPLICATE SPANS OR METRICS

Disable automatic instrumentation or the manual goslogx plugin for the affected library. Inspect nested HTTP transport and database layers and keep only one integration for the same operation.

## OTLP CONNECTION REFUSED

Confirm the hostname from the application's network namespace, the receiver port, Docker/Kubernetes service mapping, and whether the Collector is listening on `0.0.0.0:4317`.

## TLS ERRORS

Match `Insecure`, server name, root CA, and client certificate configuration to the receiver. Do not combine `Insecure` with `TLSConfig`.

## SERVICE OR APPLICATION NAME IS MISSING

Set `WithServiceName`. It becomes `application_name` in logs and `service.name` for goslogx-owned OpenTelemetry providers. Application-owned providers need their own resource configuration.

## REDIS OR DATABASE TELEMETRY IS MISSING

Verify that instrumentation is installed before the client or pool begins work, the correct logger is passed as provider, the relevant signal is enabled, filters do not reject the operation, and the provider is still running.
