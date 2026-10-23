# NET/HTTP EXAMPLE

## WHAT THE EXAMPLE DEMONSTRATES

This server installs the goslogx `net/http` middleware, propagates request context, writes correlated logs, and shuts down telemetry with the server.

## REQUIREMENTS

Go as declared by this module. The standalone server has no required external service.

## ENVIRONMENT VARIABLES

None. The server listens on `:8080`.

## HOW TO RUN

```bash
cd examples/http/std
go run .
```

## HOW TO GENERATE TRAFFIC

```bash
curl http://localhost:8080/health
```

## EXPECTED TELEMETRY AND VERIFICATION

Expect one HTTP server span, server request metrics, and logs correlated with the request span. Run `go test ./...` to verify propagation, downstream parentage, metrics, and privacy with in-memory providers.

## HOW TO SHUT IT DOWN

Send `SIGINT` or `SIGTERM`. The example stops the server before flushing and shutting down goslogx.
