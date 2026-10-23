# GIN HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server installs `goslogxgin.Middleware` and carries the active request context into application telemetry.

## REQUIREMENTS

Go as declared by this module. No external service is required.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/gin
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one Gin server span and bounded server metrics. Run `go test ./...` to verify propagation, correlation, and privacy.

## HOW TO SHUT IT DOWN

Stop the process with `Ctrl+C`; production code should stop HTTP work before telemetry shutdown.
