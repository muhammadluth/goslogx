# CHI HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server instruments Chi while preserving its resolved route template for bounded span names and metrics.

## REQUIREMENTS

Go as declared by this module. No external service is required.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/chi
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one HTTP server span with a Chi route and request metrics. Run `go test ./...` for propagation, parentage, and privacy checks.

## HOW TO SHUT IT DOWN

Stop the process with `Ctrl+C`; production code should stop HTTP work before telemetry shutdown.
