# HTTP EXAMPLES

Each framework has its own runnable server example and a downstream integration test. Run `go run .` within `std`, `fiber`, `gin`, `echo`, `chi`, or `gorillamux`; each serves `/health` on port 8080. Run `go test ./...` in an example module to verify trace propagation, log correlation and privacy across HTTP, Bun, Redis and NATS. The shared test fixture lives under `internal/testkit` and is not a plugin.

The `masking` example demonstrates logging of masked HTTP request and response data.

## REQUIREMENTS

Use the Go version declared by the selected nested module and install any service named by that example.

## ENVIRONMENT VARIABLES

Open the selected example README for its service-specific variables. Examples without variables use their documented localhost defaults.

## HOW TO RUN

Change into one of the linked example directories, then run `go run .` for executable examples or `go test ./...` for test-only modules.

## HOW TO GENERATE TRAFFIC

Use the command in the selected example README. Snippet-only examples are intended to be copied into an application and do not start a server.

## EXPECTED TELEMETRY

The selected README identifies its spans, metrics, logs, and propagation behavior.

## HOW TO VERIFY IT

Run the nested module tests where present and inspect the configured Collector/backend.

## HOW TO SHUT IT DOWN

Stop traffic and clients before flushing and shutting down the logger.
