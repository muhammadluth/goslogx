# GORILLA MUX HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server installs the Gorilla Mux plugin and uses registered route templates for HTTP telemetry.

## REQUIREMENTS

Go as declared by this module. No external service is required.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/gorillamux
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one upstream `otelmux` server span plus goslogx server metrics. Run `go test ./...` for propagation, parentage, and privacy checks.

## HOW TO SHUT IT DOWN

Stop the process with `Ctrl+C`; production code should stop HTTP work before telemetry shutdown.
