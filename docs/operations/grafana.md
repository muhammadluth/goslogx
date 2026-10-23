# GRAFANA

goslogx emits logs to its configured writer and exports managed traces and metrics through OTLP. A typical Grafana deployment routes each signal to the appropriate backend:

```text
goslogx application
    -> Alloy or OpenTelemetry Collector
       -> metrics -> Mimir
       -> logs    -> Loki
       -> traces  -> Tempo
                         -> Grafana
```

`service.name` identifies OpenTelemetry traces and metrics. `application_name` identifies goslogx log records. `obs_trace_id` correlates a log record with Tempo. `trace_id` is an application-supplied business identifier and is not interchangeable with `obs_trace_id`.

Search Loki by a bounded label such as `application_name`, then filter the log body or parsed fields by `obs_trace_id`. Never promote trace IDs, span IDs, request IDs, user IDs, or transaction IDs to Loki stream labels.
