# PRODUCTION COLLECTOR REFERENCE

This directory contains an adaptable trace pipeline for `goslogx`:

```text
application -> OTLP/gRPC receiver -> memory limiter -> resource -> batch
            -> persistent sending queue + retry -> Tempo/OTLP backend
```

[`collector.yaml`](collector.yaml) in this directory is the canonical
Collector configuration. It enables health checks, JSON internal logs,
Prometheus internal metrics, retry, and a disk-backed exporter queue. Tail
sampling policies are included but intentionally omitted from the active
pipeline.

## PORTS AND SIGNAL ROUTES

| Port | Purpose |
| --- | --- |
| `4317` | OTLP/gRPC receiver used by goslogx managed traces and metrics |
| `4318` | OTLP/HTTP receiver for compatible senders |
| `13133` | Collector health endpoint |
| `8888` | Collector internal Prometheus metrics |
| `3200` | Tempo query API in the local Compose stack |

`collector.yaml` routes traces to the configured OTLP/Tempo backend. `collector-hostmetrics.yaml` adds an application and host metrics pipeline with a separate metrics backend. These files do not configure an OTLP logs pipeline: goslogx writes JSON logs to its configured writer, and a log agent such as Alloy must collect stdout or files and send them to Loki or another log backend.

The Docker Compose file mounts the Collector and Tempo configuration from this directory and stores Tempo development data locally. The Kubernetes manifest mounts Collector configuration through a ConfigMap and uses a persistent volume for the exporter queue.

## RUNTIME AND INFRASTRUCTURE METRICS

`goslogx` does not collect CPU, memory, or network metrics. Those signals need
to come from the layer that owns the resource:

| Signal | Correct source |
| --- | --- |
| Go heap, GC, goroutines | Go runtime instrumentation in the application metrics SDK |
| Container CPU/memory and network | cAdvisor or kubelet |
| Host CPU/memory and network | node exporter or the Collector `hostmetrics` receiver |
| Collector CPU/memory and queue | Collector internal telemetry on port `8888` |
| Log records and bytes | Loki/Alloy pipeline metrics |

[`collector-hostmetrics.yaml`](collector-hostmetrics.yaml) is a
separate Contrib Collector template for host metrics and application OTLP
metrics. It has separate `traces` and `metrics` pipelines; the metrics pipeline
uses both `otlp` and `hostmetrics` receivers. Set
`OTLP_TRACE_BACKEND_ENDPOINT`/`OTLP_TRACE_BACKEND_INSECURE` for Tempo or
another trace backend, and `OTLP_METRICS_BACKEND_ENDPOINT`/
`OTLP_METRICS_BACKEND_INSECURE` for an OTLP-capable metrics backend.

Deploy `hostmetrics` as a per-node agent (normally a Kubernetes DaemonSet),
mount the host filesystem at `/hostfs`, and collect it once per node. Do not
add it to the trace gateway StatefulSet: every replica would report the same
host metrics. `hostmetrics` reports host-level signals, not per-container
usage; use cAdvisor or kubelet for the latter.

Send metrics to Prometheus, Mimir, or another metrics backend. Tempo remains
the trace backend, so it must not be used as the metrics exporter target.
Collector-internal metrics are exposed separately on port `8888`; scrape them
to alert on CPU, memory, exporter failures, and queue growth.

## LOCAL DOCKER COMPOSE

Run the Collector and a development Tempo backend:

```bash
docker compose -f deployments/otel-collector/docker-compose.yaml up -d
curl --fail http://127.0.0.1:13133/
curl --fail http://127.0.0.1:8888/metrics
```

Configure the application with endpoint `127.0.0.1:4317` and `Insecure: true`.
Tempo's query API is available at `http://127.0.0.1:3200`. The bundled Tempo
uses local storage and is intended for development, not production HA.

## PERSISTENT QUEUE AND RETRY

`file_storage` turns the exporter's sending queue into a persistent queue. It
can survive process restarts, but it is not an unlimited archive: size the
volume, alert on queue growth and failed sends, and back up or replicate the
backend independently. Remove `storage: file_storage` for an in-memory queue.
Removing the extension also requires removing it from `service.extensions`.

Important internal metrics include exporter send failures, queue capacity and
queue size. Scrape port `8888` and alert before the queue or disk fills.

## TAIL SAMPLING

To enable the supplied policies, add `tail_sampling` between
`resource/deployment` and `batch`. Tail sampling requires all spans for a trace
to reach the same sampling Collector and buffers traces until its decision
window expires. Before enabling it:

- load test memory usage and `num_traces`;
- route each trace consistently when scaling horizontally;
- decide whether errors and slow traces must always be retained;
- avoid sampling the same trace independently at multiple Collector replicas.

Head sampling in `goslogx` happens before tail sampling. A trace discarded by
head sampling can never be recovered by the Collector.

## TLS AND AUTHENTICATION

[`tls-auth-example.yaml`](tls-auth-example.yaml) demonstrates receiver mTLS,
Basic authentication, backend TLS/mTLS, and an authorization header. Mount
certificates and htpasswd files from a secret manager. Supply
`BACKEND_AUTHORIZATION` through a secret reference and never commit it.

For `goslogx`, use the Collector CA in `TracingConfig.TLSConfig` and set the
matching `authorization` header. Do not use `Insecure` outside a trusted local
network.

## KUBERNETES

[`kubernetes.yaml`](kubernetes.yaml) provides a ConfigMap, Service, hardened
StatefulSet, health probes, resource limits, and a persistent volume claim for
the queue:

```bash
kubectl create namespace observability
kubectl apply -f deployments/otel-collector/kubernetes.yaml
```

Replace the cluster name, backend endpoint, storage class, resource sizing,
and insecure backend transport before production. The example has one replica
because a disk-backed queue and tail-aware horizontal scaling require an
explicit sharding and storage design.
