# ECHO HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server installs `goslogxecho.Middleware` and preserves the active Echo request context for logs and downstream calls.

## REQUIREMENTS

Go as declared by this module. No external service is required.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/echo
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one Echo server span and request metrics. Run `go test ./...` to verify downstream parentage, propagation, and privacy.

## HOW TO SHUT IT DOWN

Stop the process with `Ctrl+C`; production code should stop HTTP work before telemetry shutdown.
