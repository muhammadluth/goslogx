# FIBER HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server installs `goslogxfiber.Middleware`, propagates request context, and emits Fiber HTTP traces and metrics without duplicating upstream middleware telemetry.

## REQUIREMENTS

Go as declared by this module. No external service is required by the standalone server.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/fiber
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one Fiber server span, request metrics, and correlated logs. Run `go test ./...` for downstream parentage, propagation, and privacy checks.

## HOW TO SHUT IT DOWN

Stop the process with `Ctrl+C`. Production applications should call Fiber shutdown before goslogx `ForceFlush` and `Shutdown`.
